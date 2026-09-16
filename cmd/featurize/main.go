// Command featurize converts collected commits (data/raw/*.jsonl.gz) into a
// sparse matrix consumed by scripts/train.py. It is a development tool and
// is not shipped.
//
// Output directory layout (little endian):
//
//	header.json      {"num_dense":..,"hash_bits":..,"feature_version":..,"rows":..}
//	X.indptr (int64) X.indices (int32) X.data (float32)   base features (no message)
//	M.indptr (int64) M.indices (int32) M.data (float32)   message-only features
//	meta.jsonl       one line per row: repo, sha, type, bot, convention, ts, nfiles, subject
//
// Column layout: [0, NumDense) dense features, then NumDense + hashed index.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"flag"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Halleck45/git-guess/internal/diff"
	"github.com/Halleck45/git-guess/internal/features"
)

type record struct {
	Repo       string `json:"repo"`
	Sha        string `json:"sha"`
	Type       string `json:"type"`
	Scope      string `json:"scope"`
	Subject    string `json:"subject"`
	Convention string `json:"convention"`
	Bot        bool   `json:"bot"`
	Ts         int64  `json:"ts"`
	NFiles     int    `json:"nfiles"`
	PatchHash  string `json:"patch_hash"`
	Patch      string `json:"patch"`
}

type meta struct {
	Repo       string `json:"repo"`
	Sha        string `json:"sha"`
	Type       string `json:"type"`
	Bot        bool   `json:"bot"`
	Convention string `json:"convention"`
	Ts         int64  `json:"ts"`
	NFiles     int    `json:"nfiles"`
	Subject    string `json:"subject"`
}

type row struct {
	meta meta
	base *features.Vector
	msg  []features.Sparse
}

type writer struct {
	xIndptr, xIndices, xData *bufio.Writer
	mIndptr, mIndices, mData *bufio.Writer
	meta                     *bufio.Writer
	xNNZ, mNNZ               int64
	rows                     int64
	files                    []*os.File
}

func newWriter(dir string) *writer {
	w := &writer{}
	open := func(name string) *bufio.Writer {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			log.Fatal(err)
		}
		w.files = append(w.files, f)
		return bufio.NewWriterSize(f, 1<<20)
	}
	w.xIndptr, w.xIndices, w.xData = open("X.indptr"), open("X.indices"), open("X.data")
	w.mIndptr, w.mIndices, w.mData = open("M.indptr"), open("M.indices"), open("M.data")
	w.meta = open("meta.jsonl")
	binary.Write(w.xIndptr, binary.LittleEndian, int64(0))
	binary.Write(w.mIndptr, binary.LittleEndian, int64(0))
	return w
}

func (w *writer) write(r row) {
	var buf [8]byte
	put32 := func(bw *bufio.Writer, v uint32) { binary.LittleEndian.PutUint32(buf[:4], v); bw.Write(buf[:4]) }
	putf := func(bw *bufio.Writer, v float32) {
		binary.LittleEndian.PutUint32(buf[:4], mathFloat32bits(v))
		bw.Write(buf[:4])
	}
	put64 := func(bw *bufio.Writer, v int64) { binary.LittleEndian.PutUint64(buf[:8], uint64(v)); bw.Write(buf[:8]) }
	for i, v := range r.base.Dense {
		if v != 0 {
			put32(w.xIndices, uint32(i))
			putf(w.xData, v)
			w.xNNZ++
		}
	}
	for _, s := range r.base.Sparse {
		put32(w.xIndices, uint32(features.NumDense)+s.Index)
		putf(w.xData, s.Value)
		w.xNNZ++
	}
	put64(w.xIndptr, w.xNNZ)
	for _, s := range r.msg {
		put32(w.mIndices, uint32(features.NumDense)+s.Index)
		putf(w.mData, s.Value)
		w.mNNZ++
	}
	put64(w.mIndptr, w.mNNZ)
	b, _ := json.Marshal(r.meta)
	w.meta.Write(b)
	w.meta.WriteByte('\n')
	w.rows++
}

func (w *writer) close(dir string) {
	for _, bw := range []*bufio.Writer{w.xIndptr, w.xIndices, w.xData, w.mIndptr, w.mIndices, w.mData, w.meta} {
		bw.Flush()
	}
	for _, f := range w.files {
		f.Close()
	}
	h := map[string]any{"num_dense": features.NumDense, "hash_bits": features.HashBits, "feature_version": features.Version,
		"rows": w.rows, "x_nnz": w.xNNZ, "m_nnz": w.mNNZ, "dense_names": features.DenseNames}
	b, _ := json.MarshalIndent(h, "", " ")
	os.WriteFile(filepath.Join(dir, "header.json"), b, 0o644)
}

func main() {
	in := flag.String("in", "data/raw", "input directory of *.jsonl.gz")
	out := flag.String("out", "data/features", "output directory")
	maxPerRepo := flag.Int("max-per-repo", 4000, "max non-bot commits per repo")
	maxBotPerRepo := flag.Int("max-bot-per-repo", 150, "max bot commits per repo")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel workers")
	hashBits := flag.Int("hash-bits", features.HashBits, "size of the hashed feature space (2^bits)")
	bigrams := flag.Bool("bigrams", false, "enable code token bigrams")
	flag.Parse()
	features.HashBits = *hashBits
	features.CodeBigrams = *bigrams
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(*in, "*.jsonl.gz"))
	sort.Strings(paths)
	log.Printf("%d input files", len(paths))

	// Pass 1: read metadata, dedupe, cap per repo. We keep the full record in
	// memory only for the files being processed (workers stream files).
	seen := map[string]bool{}
	var seenMu sync.Mutex
	type job struct{ path string }
	jobs := make(chan job)
	results := make(chan row, 1024)
	var wg sync.WaitGroup
	stats := map[string]int{}
	var statsMu sync.Mutex
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				processFile(j.path, *maxPerRepo, *maxBotPerRepo, seen, &seenMu, results, stats, &statsMu)
			}
		}()
	}
	w := newWriter(*out)
	done := make(chan struct{})
	go func() {
		for r := range results {
			w.write(r)
			if w.rows%20000 == 0 {
				log.Printf("rows=%d nnz=%d", w.rows, w.xNNZ)
			}
		}
		close(done)
	}()
	for _, p := range paths {
		jobs <- job{p}
	}
	close(jobs)
	wg.Wait()
	close(results)
	<-done
	w.close(*out)
	log.Printf("done: rows=%d x_nnz=%d m_nnz=%d skipped=%v", w.rows, w.xNNZ, w.mNNZ, stats)
}

func processFile(p string, maxPerRepo, maxBot int, seen map[string]bool, seenMu *sync.Mutex, out chan<- row, stats map[string]int, statsMu *sync.Mutex) {
	f, err := os.Open(p)
	if err != nil {
		log.Printf("%s: %v", p, err)
		return
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		log.Printf("%s: %v", p, err)
		return
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var recs []record
	for sc.Scan() {
		var r record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		recs = append(recs, r)
	}
	// Deterministic shuffle by sha so caps do not favour recent commits only.
	sort.Slice(recs, func(i, j int) bool { return hashSha(recs[i].Sha) < hashSha(recs[j].Sha) })
	nHuman, nBot := 0, 0
	local := map[string]int{}
	for _, r := range recs {
		if r.Bot {
			if nBot >= maxBot {
				local["cap_bot"]++
				continue
			}
		} else if nHuman >= maxPerRepo {
			local["cap_repo"]++
			continue
		}
		seenMu.Lock()
		dup := seen[r.PatchHash]
		seen[r.PatchHash] = true
		seenMu.Unlock()
		if dup {
			local["dup"]++
			continue
		}
		d := diff.ParseString(r.Patch)
		if len(d.Files) == 0 {
			local["empty"]++
			continue
		}
		base, msg := features.ExtractParts(d, r.Subject)
		if r.Bot {
			nBot++
		} else {
			nHuman++
		}
		out <- row{meta: meta{Repo: r.Repo, Sha: r.Sha, Type: r.Type, Bot: r.Bot, Convention: r.Convention, Ts: r.Ts, NFiles: r.NFiles, Subject: r.Subject}, base: base, msg: msg}
	}
	statsMu.Lock()
	for k, v := range local {
		stats[k] += v
	}
	statsMu.Unlock()
	log.Printf("%s: kept=%d bot=%d skipped=%v", strings.TrimSuffix(filepath.Base(p), ".jsonl.gz"), nHuman, nBot, local)
}

func hashSha(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
