// Package history indexes the repository's own conventional commits so that
// predictions can be adapted to local habits: nearest neighbours among past
// diffs, a confusion correction (what this repository calls what the global
// model guesses), and per-file history.
//
// The index lives in <gitdir>/conventional/history.bin and is refreshed
// incrementally: only commits not yet indexed are diffed and featurized.
package history

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Halleck45/git-guess/internal/diff"
	"github.com/Halleck45/git-guess/internal/features"
	"github.com/Halleck45/git-guess/internal/gitx"
	"github.com/Halleck45/git-guess/internal/model"
)

// Window is the number of recent conventional commits kept in the index.
const Window = 1000

// MinEntries is the history size below which no adaptation is attempted.
const MinEntries = 20

const cacheVersion = 1

// Entry is one indexed commit.
type Entry struct {
	Sha    string
	Ts     int64
	Type   uint8
	Paths  []string
	Logits []float32
	Vec    []features.Sparse // hashed features, L2-normalised
}

// Index is the ordered (oldest first) history of a repository.
type Index struct {
	Version    uint32
	FeatureVer uint32
	Entries    []Entry
}

type onDisk struct {
	Version    uint32
	FeatureVer uint32
	Entries    []Entry
}

func cachePath() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(string(out))
	return filepath.Join(dir, "git-guess", "history.bin"), nil
}

func loadCache(p string) *Index {
	f, err := os.Open(p)
	if err != nil {
		return &Index{Version: cacheVersion, FeatureVer: features.Version}
	}
	defer f.Close()
	var d onDisk
	if err := gob.NewDecoder(bufio.NewReader(f)).Decode(&d); err != nil || d.Version != cacheVersion || d.FeatureVer != features.Version {
		return &Index{Version: cacheVersion, FeatureVer: features.Version}
	}
	return &Index{Version: d.Version, FeatureVer: d.FeatureVer, Entries: d.Entries}
}

func saveCache(p string, idx *Index) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if err := gob.NewEncoder(w).Encode(onDisk{Version: idx.Version, FeatureVer: idx.FeatureVer, Entries: idx.Entries}); err != nil {
		f.Close()
		return err
	}
	w.Flush()
	f.Close()
	return os.Rename(tmp, p)
}

// Progress is called while indexing commits (done, total).
type Progress func(done, total int)

// Load returns the up-to-date index of the current repository, refreshing it
// when new commits appeared. skip ignores the most recent commits (used by
// eval so that the index only contains older history).
func Load(m *model.Model, skip int, progress Progress) (*Index, error) {
	p, err := cachePath()
	if err != nil {
		return nil, err
	}
	idx := loadCache(p)
	// Recent conventional commits, newest first.
	out, err := exec.Command("git", "log", "--no-merges", "--format=%H%x00%at%x00%s", fmt.Sprintf("--skip=%d", skip), fmt.Sprintf("-n%d", Window*2)).Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	type want struct {
		sha string
		ts  int64
		typ uint8
	}
	var wanted []want
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		typ, _, ok := gitx.ParseHeader(parts[2], m.Classes)
		if !ok {
			continue
		}
		ts, _ := strconv.ParseInt(parts[1], 10, 64)
		wanted = append(wanted, want{parts[0], ts, uint8(m.ClassIndex(typ))})
		if len(wanted) >= Window {
			break
		}
	}
	have := make(map[string]*Entry, len(idx.Entries))
	for i := range idx.Entries {
		have[idx.Entries[i].Sha] = &idx.Entries[i]
	}
	var missing []string
	for _, w := range wanted {
		if _, ok := have[w.sha]; !ok {
			missing = append(missing, w.sha)
		}
	}
	if len(missing) > 0 {
		fresh, err := featurizeCommits(m, missing, progress)
		if err != nil {
			return nil, err
		}
		for sha, e := range fresh {
			have[sha] = e
		}
	}
	// Rebuild the window, oldest first.
	entries := make([]Entry, 0, len(wanted))
	for i := len(wanted) - 1; i >= 0; i-- {
		w := wanted[i]
		e, ok := have[w.sha]
		if !ok {
			continue
		}
		e.Ts, e.Type = w.ts, w.typ
		entries = append(entries, *e)
	}
	idx.Entries = entries
	if len(missing) > 0 {
		_ = saveCache(p, idx) // best effort
	}
	return idx, nil
}

// featurizeCommits diffs and featurizes commits in batches with one git
// process per batch.
func featurizeCommits(m *model.Model, shas []string, progress Progress) (map[string]*Entry, error) {
	out := make(map[string]*Entry, len(shas))
	const batch = 200
	done := 0
	for start := 0; start < len(shas); start += batch {
		end := min(start+batch, len(shas))
		args := []string{"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false", "-c", "core.quotepath=false",
			"log", "--no-walk=unsorted", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--unified=3", "--diff-merges=first-parent",
			"--format=%x1e%H", "-p"}
		args = append(args, shas[start:end]...)
		cmd := exec.Command("git", args...)
		cmd.Stderr = io.Discard
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		if err := splitCommits(stdout, func(sha string, patch []byte) {
			d, _ := diff.Parse(bytes.NewReader(patch))
			if len(d.Files) == 0 {
				return
			}
			out[sha] = newEntry(m, sha, d)
			done++
			if progress != nil {
				progress(done, len(shas))
			}
		}); err != nil {
			cmd.Wait()
			return nil, err
		}
		cmd.Wait()
	}
	return out, nil
}

// splitCommits streams "\x1e<sha>\n<patch>" records.
func splitCommits(r io.Reader, fn func(sha string, patch []byte)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var sha string
	var buf bytes.Buffer
	flush := func() {
		if sha != "" {
			fn(sha, buf.Bytes())
		}
		buf.Reset()
	}
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) > 0 && line[0] == 0x1e {
			flush()
			sha = strings.TrimSpace(string(line[1:]))
			continue
		}
		if buf.Len() < 8<<20 { // cap pathological commits
			buf.Write(line)
			buf.WriteByte('\n')
		}
	}
	flush()
	return sc.Err()
}

func newEntry(m *model.Model, sha string, d *diff.Diff) *Entry {
	v := features.Extract(d, features.Options{})
	z := m.Logits(v)
	e := &Entry{Sha: sha, Logits: make([]float32, len(z)), Vec: Normalize(v.Sparse)}
	for i, x := range z {
		e.Logits[i] = float32(x)
	}
	for i, f := range d.Files {
		if i >= 50 {
			break
		}
		e.Paths = append(e.Paths, f.Path)
	}
	return e
}

// Normalize returns a copy of the sparse vector scaled to unit L2 norm.
func Normalize(s []features.Sparse) []features.Sparse {
	var n float64
	for _, x := range s {
		n += float64(x.Value) * float64(x.Value)
	}
	n = math.Sqrt(n)
	if n == 0 {
		n = 1
	}
	out := make([]features.Sparse, len(s))
	for i, x := range s {
		out[i] = features.Sparse{Index: x.Index, Value: float32(float64(x.Value) / n)}
	}
	return out
}

// Dot of two sorted sparse vectors.
func Dot(a, b []features.Sparse) float64 {
	var s float64
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i].Index < b[j].Index:
			i++
		case a[i].Index > b[j].Index:
			j++
		default:
			s += float64(a[i].Value) * float64(b[j].Value)
			i++
			j++
		}
	}
	return s
}

// Evidence gathered from the history for one prediction.
type Evidence struct {
	KNN     []float64 // smoothed type distribution of the nearest diffs
	Conf    []float64 // global probabilities corrected by the local confusion
	File    []float64 // smoothed type distribution of past commits touching the same files
	Seen    bool      // at least one touched file has history
	Size    int       // history entries used
	MaxSim  float64   // similarity of the closest diff
	Nearest []Neighbour
}

// Neighbour is one of the closest past commits.
type Neighbour struct {
	Sha  string
	Type string
	Sim  float64
}

const (
	knnK        = 15
	knnSmooth   = 0.3
	confDiag    = 2.0
	confSmooth  = 0.2
	fileSmooth  = 0.05
	temperature = 1.5
)

// Gather computes the local evidence for a diff (hashed features `sparse`,
// touched `paths`) whose global logits are z. Only entries[:limit] are used
// (limit < 0 means all).
func (idx *Index) Gather(m *model.Model, sparse []features.Sparse, paths []string, z []float64, limit int) *Evidence {
	entries := idx.Entries
	if limit >= 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	if len(entries) > Window {
		entries = entries[len(entries)-Window:]
	}
	C := len(m.Classes)
	ev := &Evidence{Size: len(entries)}
	p := m.Probabilities(z)
	// --- kNN
	q := Normalize(sparse)
	type scored struct {
		i   int
		sim float64
	}
	sims := make([]scored, len(entries))
	for i := range entries {
		sims[i] = scored{i, Dot(q, entries[i].Vec)}
	}
	sort.Slice(sims, func(a, b int) bool { return sims[a].sim > sims[b].sim })
	k := min(knnK, len(sims))
	kv := make([]float64, C)
	var ksum float64
	for _, s := range sims[:k] {
		w := math.Max(s.sim, 0)
		w *= w
		kv[entries[s.i].Type] += w
		ksum += w
		ev.Nearest = append(ev.Nearest, Neighbour{Sha: entries[s.i].Sha, Type: m.Classes[entries[s.i].Type], Sim: s.sim})
	}
	if len(sims) > 0 {
		ev.MaxSim = sims[0].sim
	}
	ev.KNN = make([]float64, C)
	for c := range kv {
		ev.KNN[c] = (kv[c] + knnSmooth) / (ksum + knnSmooth*float64(C))
	}
	// --- confusion correction
	conf := make([]float64, C*C)
	for i := range entries {
		g := argmax32(entries[i].Logits)
		conf[g*C+int(entries[i].Type)]++
	}
	ev.Conf = make([]float64, C)
	for g := 0; g < C; g++ {
		var row float64
		for t := 0; t < C; t++ {
			x := conf[g*C+t] + confSmooth
			if g == t {
				x += confDiag
			}
			row += x
		}
		for t := 0; t < C; t++ {
			x := conf[g*C+t] + confSmooth
			if g == t {
				x += confDiag
			}
			ev.Conf[t] += p[g] * x / row
		}
	}
	// --- per-file history
	fileCounts := map[string][]float64{}
	for i := range entries {
		for _, pth := range entries[i].Paths {
			c := fileCounts[pth]
			if c == nil {
				c = make([]float64, C)
				fileCounts[pth] = c
			}
			c[entries[i].Type]++
		}
	}
	fp := make([]float64, C)
	seen := 0
	for i, pth := range paths {
		if i >= 50 {
			break
		}
		c, ok := fileCounts[pth]
		if !ok {
			continue
		}
		var s float64
		for _, x := range c {
			s += x
		}
		for t := range fp {
			fp[t] += c[t] / s
		}
		seen++
	}
	ev.File = make([]float64, C)
	if seen > 0 {
		ev.Seen = true
		for t := range fp {
			ev.File[t] = (fp[t]/float64(seen) + fileSmooth) / (1 + fileSmooth*float64(C))
		}
	} else {
		// marginal distribution of the history
		counts := make([]float64, C)
		for i := range entries {
			counts[entries[i].Type]++
		}
		for t := range counts {
			ev.File[t] = (counts[t] + 1) / (float64(len(entries)) + float64(C))
		}
	}
	return ev
}

func argmax32(z []float32) int {
	best := 0
	for i, x := range z {
		if x > z[best] {
			best = i
		}
	}
	return best
}
