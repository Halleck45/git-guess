// Package features turns a parsed diff (and an optional commit message) into
// a fixed-size dense vector plus a sparse hashed vector. It is the single
// source of truth for the model input: the training pipeline calls Extract
// on stored patches, the CLI calls it on `git diff` output.
//
// Any change in this package invalidates trained models; bump Version.
package features

import (
	"hash/fnv"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/Halleck45/git-guess/internal/diff"
)

// Version of the feature contract. Stored in the model file and checked at
// load time.
const Version = 2

// HashBits is the size of the hashed feature space (2^HashBits buckets). It
// is a variable only so that the training pipeline can experiment; the
// value is stored in the model file and checked at load time.
var HashBits = 20

// CodeBigrams toggles the token bigram features of changed lines. Experiment
// knob for the training pipeline; part of the model contract.
var CodeBigrams = false

// Dense feature indices. Keep DenseNames in sync.
const (
	dLogFiles = iota
	dLogAdded
	dLogRemoved
	dAddRatio
	dLogTotal
	dFracNew
	dFracDeleted
	dFracRenamed
	dFracBinary
	dFracCat // numCategories slots: fraction of files per category
)

const dOnlyCat = dFracCat + int(numCategories) // numCategories slots: all files in category

const (
	dWSSim = dOnlyCat + int(numCategories) + iota
	dTokJaccard
	dCommentAdded
	dCommentRemoved
	dImportAdded
	dImportRemoved
	dPureAddFiles
	dPureDelFiles
	dVersionBump
	dNumLangs
	dMaxFileShare
	dHasMessage
	dLinesInTests
	dLinesInDocs
	dLinesInSource
	dTruncated
	dSingleFile
	dLogHunks
	dModeChange
	dAvgLineLen
	dTrivialAdded
	dTestKeywords
	dSameExt
	dLinesInCI
	dLinesInBuild
	dLinesInConfig
	dLinesInStyle
	dLinesInSchema
	dLinesInGenerated
	dLinesInI18n
	dLinesInChangelog
	dStringsAdded
	dLogAddedChanged
	dErrorKeywords
	dNullCheckKeywords
	dLogKeywords
	dDeprecatedKeywords
	dTypeKeywords
	dOnlyAdditions
	dOnlyDeletions
	dFilesTouchingTestsAndSource
	NumDense
)

// DenseNames gives a human readable name to each dense feature.
var DenseNames = func() []string {
	n := make([]string, NumDense)
	set := func(i int, s string) { n[i] = s }
	set(dLogFiles, "files")
	set(dLogAdded, "added_lines")
	set(dLogRemoved, "removed_lines")
	set(dAddRatio, "add_ratio")
	set(dLogTotal, "total_lines")
	set(dFracNew, "new_files")
	set(dFracDeleted, "deleted_files")
	set(dFracRenamed, "renamed_files")
	set(dFracBinary, "binary_files")
	for c := 0; c < int(numCategories); c++ {
		set(dFracCat+c, "files:"+Category(c).String())
		set(dOnlyCat+c, "only:"+Category(c).String())
	}
	set(dWSSim, "whitespace_only_similarity")
	set(dTokJaccard, "token_overlap_added_removed")
	set(dCommentAdded, "comments_added")
	set(dCommentRemoved, "comments_removed")
	set(dImportAdded, "imports_added")
	set(dImportRemoved, "imports_removed")
	set(dPureAddFiles, "files_pure_additions")
	set(dPureDelFiles, "files_pure_deletions")
	set(dVersionBump, "version_bump")
	set(dNumLangs, "languages")
	set(dMaxFileShare, "largest_file_share")
	set(dHasMessage, "has_message")
	set(dLinesInTests, "lines_in_tests")
	set(dLinesInDocs, "lines_in_docs")
	set(dLinesInSource, "lines_in_source")
	set(dTruncated, "truncated")
	set(dSingleFile, "single_file")
	set(dLogHunks, "hunks")
	set(dModeChange, "mode_change")
	set(dAvgLineLen, "avg_added_line_len")
	set(dTrivialAdded, "trivial_added_lines")
	set(dTestKeywords, "test_keywords")
	set(dSameExt, "same_extension")
	set(dLinesInCI, "lines_in_ci")
	set(dLinesInBuild, "lines_in_build")
	set(dLinesInConfig, "lines_in_config")
	set(dLinesInStyle, "lines_in_style")
	set(dLinesInSchema, "lines_in_schema")
	set(dLinesInGenerated, "lines_in_generated")
	set(dLinesInI18n, "lines_in_i18n")
	set(dLinesInChangelog, "lines_in_changelog")
	set(dStringsAdded, "string_literals_added")
	set(dLogAddedChanged, "added_minus_removed")
	set(dErrorKeywords, "error_handling_keywords")
	set(dNullCheckKeywords, "null_check_keywords")
	set(dLogKeywords, "logging_keywords")
	set(dDeprecatedKeywords, "deprecation_keywords")
	set(dTypeKeywords, "type_annotation_keywords")
	set(dOnlyAdditions, "only_additions")
	set(dOnlyDeletions, "only_deletions")
	set(dFilesTouchingTestsAndSource, "tests_and_source")
	return n
}()

// Sparse is one hashed feature.
type Sparse struct {
	Index uint32
	Value float32
}

// Vector is the model input.
type Vector struct {
	Dense  []float32
	Sparse []Sparse // sorted by Index, unique
	Names  []string // optional, aligned with Sparse (only when Options.KeepNames)
}

// Options for Extract.
type Options struct {
	Message   string // optional draft commit subject; empty means unknown
	KeepNames bool   // keep feature names for explanations
}

// group accumulates counts for one feature group, normalized separately.
type group struct {
	counts map[string]float32
}

func newGroup() *group                    { return &group{counts: make(map[string]float32, 256)} }
func (g *group) add(k string)             { g.counts[k]++ }
func (g *group) addN(k string, n float32) { g.counts[k] += n }

// Hash maps a feature name to a bucket.
func Hash(name string) uint32 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return uint32(h.Sum64() & (1<<uint(HashBits) - 1))
}

func clamp01(x float64) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return float32(x)
}

func logScale(n, max float64) float32 { return clamp01(math.Log1p(n) / math.Log1p(max)) }

func frac(a, b int) float32 {
	if b == 0 {
		return 0
	}
	return float32(a) / float32(b)
}

// Extract computes the feature vector of a diff.
func Extract(d *diff.Diff, opts Options) *Vector {
	v, _ := extract(d, opts)
	return v
}

// ExtractParts returns the vector computed WITHOUT the message, plus the
// message-only sparse part (already normalized). Adding the part to the
// vector and setting the has_message dense flag gives exactly what Extract
// returns with the message. Used by the training pipeline to train with
// message dropout from a single featurization pass.
func ExtractParts(d *diff.Diff, message string) (base *Vector, msg []Sparse) {
	base, _ = extract(d, Options{})
	_, msg = extract(&diff.Diff{}, Options{Message: message})
	return base, msg
}

// HasMessageIndex is the dense index of the has_message flag.
const HasMessageIndex = dHasMessage

func extract(d *diff.Diff, opts Options) (*Vector, []Sparse) {
	dense := make([]float32, NumDense)
	gPath, gCode, gShape, gHunk, gMsg := newGroup(), newGroup(), newGroup(), newGroup(), newGroup()

	nFiles := len(d.Files)
	var added, removed, newF, delF, renF, binF, hunks, modeChg int
	catFiles := make([]int, numCategories)
	catLines := make([]int, numCategories)
	exts := map[string]int{}
	langs := map[string]int{}
	pureAdd, pureDel := 0, 0
	maxFileLines := 0
	var commentAdd, commentRem, importAdd, importRem, trivialAdd, testKw, strAdd, errKw, nullKw, logKw, deprKw, typeKw int
	var addedLen int
	versionBump := false
	normRemoved := map[string]int{}
	normAdded := map[string]int{}
	tokAdded := map[string]struct{}{}
	tokRemoved := map[string]struct{}{}
	var toks, prevToks []string

	for fi := range d.Files {
		f := &d.Files[fi]
		lp := strings.ToLower(f.Path)
		base := path.Base(lp)
		ext := extOf(base)
		lang := Lang(ext)
		cat := Classify(f.Path)
		catFiles[cat]++
		catLines[cat] += f.Added + f.Removed
		exts[ext]++
		langs[lang]++
		added += f.Added
		removed += f.Removed
		hunks += len(f.Hunks)
		if f.Added+f.Removed > maxFileLines {
			maxFileLines = f.Added + f.Removed
		}
		if f.Mode == "100755" || f.Mode == "755" {
			modeChg++
		}
		switch f.Status {
		case diff.Added:
			newF++
		case diff.Deleted:
			delF++
		case diff.Renamed, diff.Copied:
			renF++
		}
		if f.Binary {
			binF++
		}
		if f.Removed == 0 && f.Added > 0 {
			pureAdd++
		}
		if f.Added == 0 && f.Removed > 0 {
			pureDel++
		}
		st := f.Status.String()

		// ---- path features
		dir := path.Dir(lp)
		segs := []string{}
		if dir != "." && dir != "" {
			segs = strings.Split(dir, "/")
		}
		for i, s := range segs {
			gPath.add("p:" + s)
			if i == 0 {
				gPath.add("p0:" + s)
			}
			if i+1 < len(segs) {
				gPath.add("pp:" + s + "/" + segs[i+1])
			}
		}
		if len(segs) > 0 {
			gPath.add("pl:" + segs[len(segs)-1])
			gPath.add("fe:" + segs[len(segs)-1] + "/" + ext)
		} else {
			gPath.add("pl:<root>")
			gPath.add("fe:<root>/" + ext)
		}
		gPath.add("f:" + base)
		toks = tokenizeInto(toks[:0], strings.TrimSuffix(base, "."+ext))
		for _, t := range toks {
			gPath.add("fw:" + t)
		}
		gPath.add("e:" + ext)
		gPath.add("l:" + lang)
		gPath.add("c:" + cat.String())
		gPath.add("s:" + st)
		gPath.add("cs:" + cat.String() + "_" + st)
		gPath.add("es:" + ext + "_" + st)
		gPath.add("ce:" + cat.String() + "_" + ext)
		gPath.add("ls:" + lang + "_" + st)
		if f.Binary {
			gPath.add("bin")
		}
		if f.OldPath != "" {
			od := path.Dir(strings.ToLower(f.OldPath))
			if od == dir {
				gPath.add("ren:samedir")
			} else {
				gPath.add("ren:moved")
			}
			if extOf(path.Base(strings.ToLower(f.OldPath))) != ext {
				gPath.add("ren:ext")
			}
		}

		// ---- hunk context
		for _, h := range f.Hunks {
			toks = tokenizeInto(toks[:0], h)
			for _, t := range toks {
				gHunk.add("h:" + t)
			}
			gHunk.add("hs:" + lineShape(h))
		}

		// ---- content
		fileIsBuild := cat == CatBuild || cat == CatVersion || cat == CatLock || cat == CatConfig
		verAdd, verRem := false, false
		prevToks = prevToks[:0]
		for li := range f.Lines {
			l := &f.Lines[li]
			text := l.Text
			trim := strings.TrimSpace(text)
			if len(trim) > 400 { // minified / generated line: keep only shape
				if l.Added {
					gShape.add("A1:<long>")
				} else {
					gShape.add("R1:<long>")
				}
				continue
			}
			shape := lineShape(trim)
			lower := strings.ToLower(trim)
			if l.Added {
				gShape.add("A1:" + shape)
				gShape.add("AL:" + lang + ":" + shape)
				addedLen += len(trim)
				if trim == "" || len(trim) <= 2 && !isAlnum(trim) {
					trivialAdd++
				}
				if shape == "<comment>" {
					commentAdd++
				}
				if isImportLine(trim) {
					importAdd++
				}
				if hasTestKeyword(lower) {
					testKw++
				}
				if strings.ContainsAny(trim, "\"'`") {
					strAdd++
				}
				if hasErrorKeyword(lower) {
					errKw++
				}
				if hasNullCheck(lower) {
					nullKw++
				}
				if hasLogKeyword(lower) {
					logKw++
				}
				if strings.Contains(lower, "deprecat") {
					deprKw++
				}
				if hasTypeKeyword(lower) {
					typeKw++
				}
				if fileIsBuild && strings.Contains(lower, "version") && strings.ContainsAny(lower, "0123456789") {
					verAdd = true
				}
				normAdded[normalizeWS(trim)]++
			} else {
				gShape.add("R1:" + shape)
				gShape.add("RL:" + lang + ":" + shape)
				if shape == "<comment>" {
					commentRem++
				}
				if isImportLine(trim) {
					importRem++
				}
				if strings.Contains(lower, "deprecat") {
					deprKw++
				}
				if fileIsBuild && strings.Contains(lower, "version") && strings.ContainsAny(lower, "0123456789") {
					verRem = true
				}
				normRemoved[normalizeWS(trim)]++
			}
			toks = tokenizeInto(toks[:0], trim)
			if l.Added {
				for i, t := range toks {
					gCode.add("a:" + t)
					tokAdded[t] = struct{}{}
					if i > 0 && CodeBigrams {
						gCode.add("ab:" + toks[i-1] + "_" + t)
					}
				}
			} else {
				for i, t := range toks {
					gCode.add("r:" + t)
					tokRemoved[t] = struct{}{}
					if i > 0 && CodeBigrams {
						gCode.add("rb:" + toks[i-1] + "_" + t)
					}
				}
			}
		}
		if verAdd && verRem {
			versionBump = true
		}
	}

	// ---- symmetric difference of vocabularies
	for t := range tokAdded {
		if _, ok := tokRemoved[t]; !ok {
			gCode.add("x:" + t)
		}
	}
	for t := range tokRemoved {
		if _, ok := tokAdded[t]; !ok {
			gCode.add("y:" + t)
		}
	}
	// ---- whitespace-only similarity
	match, totalNorm := 0, 0
	for k, n := range normRemoved {
		totalNorm += n
		if m, ok := normAdded[k]; ok {
			if m < n {
				match += m
			} else {
				match += n
			}
		}
	}
	for _, n := range normAdded {
		totalNorm += n
	}
	if totalNorm > 0 {
		dense[dWSSim] = float32(2*match) / float32(totalNorm)
	}
	// token jaccard
	inter, union := 0, len(tokAdded)
	for t := range tokRemoved {
		if _, ok := tokAdded[t]; ok {
			inter++
		} else {
			union++
		}
	}
	if union > 0 {
		dense[dTokJaccard] = float32(inter) / float32(union)
	}

	// ---- dense
	total := added + removed
	dense[dLogFiles] = logScale(float64(nFiles), 200)
	dense[dLogAdded] = logScale(float64(added), 20000)
	dense[dLogRemoved] = logScale(float64(removed), 20000)
	dense[dAddRatio] = frac(added, total)
	dense[dLogTotal] = logScale(float64(total), 40000)
	dense[dFracNew] = frac(newF, nFiles)
	dense[dFracDeleted] = frac(delF, nFiles)
	dense[dFracRenamed] = frac(renF, nFiles)
	dense[dFracBinary] = frac(binF, nFiles)
	for c := 0; c < int(numCategories); c++ {
		dense[dFracCat+c] = frac(catFiles[c], nFiles)
		if nFiles > 0 && catFiles[c] == nFiles {
			dense[dOnlyCat+c] = 1
		}
	}
	dense[dCommentAdded] = frac(commentAdd, added)
	dense[dCommentRemoved] = frac(commentRem, removed)
	dense[dImportAdded] = frac(importAdd, added)
	dense[dImportRemoved] = frac(importRem, removed)
	dense[dPureAddFiles] = frac(pureAdd, nFiles)
	dense[dPureDelFiles] = frac(pureDel, nFiles)
	if versionBump {
		dense[dVersionBump] = 1
	}
	dense[dNumLangs] = clamp01(float64(len(langs)) / 5)
	dense[dMaxFileShare] = frac(maxFileLines, total)
	if opts.Message != "" {
		dense[dHasMessage] = 1
	}
	dense[dLinesInTests] = frac(catLines[CatTest]+catLines[CatSnapshot], total)
	dense[dLinesInDocs] = frac(catLines[CatDocs], total)
	dense[dLinesInSource] = frac(catLines[CatSource], total)
	if d.Truncated {
		dense[dTruncated] = 1
	}
	if nFiles == 1 {
		dense[dSingleFile] = 1
	}
	dense[dLogHunks] = logScale(float64(hunks), 2000)
	dense[dModeChange] = frac(modeChg, nFiles)
	if added > 0 {
		dense[dAvgLineLen] = clamp01(float64(addedLen) / float64(added) / 120)
	}
	dense[dTrivialAdded] = frac(trivialAdd, added)
	dense[dTestKeywords] = frac(testKw, added)
	if len(exts) == 1 {
		dense[dSameExt] = 1
	}
	dense[dLinesInCI] = frac(catLines[CatCI], total)
	dense[dLinesInBuild] = frac(catLines[CatBuild]+catLines[CatLock]+catLines[CatVersion], total)
	dense[dLinesInConfig] = frac(catLines[CatConfig], total)
	dense[dLinesInStyle] = frac(catLines[CatStyle], total)
	dense[dLinesInSchema] = frac(catLines[CatSchema], total)
	dense[dLinesInGenerated] = frac(catLines[CatGenerated], total)
	dense[dLinesInI18n] = frac(catLines[CatI18n], total)
	dense[dLinesInChangelog] = frac(catLines[CatChangelog], total)
	dense[dStringsAdded] = frac(strAdd, added)
	dense[dLogAddedChanged] = clamp01(0.5 + float64(added-removed)/float64(max(total, 1))/2)
	dense[dErrorKeywords] = frac(errKw, added)
	dense[dNullCheckKeywords] = frac(nullKw, added)
	dense[dLogKeywords] = frac(logKw, added)
	dense[dDeprecatedKeywords] = clamp01(float64(deprKw) / 3)
	dense[dTypeKeywords] = frac(typeKw, added)
	if removed == 0 && added > 0 {
		dense[dOnlyAdditions] = 1
	}
	if added == 0 && removed > 0 {
		dense[dOnlyDeletions] = 1
	}
	if catFiles[CatTest] > 0 && catFiles[CatSource] > 0 {
		dense[dFilesTouchingTestsAndSource] = 1
	}

	// ---- message
	if opts.Message != "" {
		msg := opts.Message
		if i := strings.LastIndex(msg, "(#"); i > 0 {
			msg = msg[:i]
		}
		toks = tokenizeInto(toks[:0], msg)
		for i, t := range toks {
			if i == 0 {
				gMsg.add("m1:" + t)
			}
			gMsg.add("m:" + t)
			if i > 0 {
				gMsg.add("m2:" + toks[i-1] + "_" + t)
			}
		}
		if len(toks) == 0 {
			gMsg.add("m:<empty>")
		}
	}

	// ---- hash & normalize
	v := &Vector{Dense: dense}
	acc := make(map[uint32]float32, 512)
	var names map[uint32]string
	if opts.KeepNames {
		names = make(map[uint32]string, 512)
	}
	for _, g := range []*group{gPath, gCode, gShape, gHunk, gMsg} {
		if len(g.counts) == 0 {
			continue
		}
		var norm float64
		for _, c := range g.counts {
			l := math.Log1p(float64(c))
			norm += l * l
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		for k, c := range g.counts {
			idx := Hash(k)
			acc[idx] += float32(math.Log1p(float64(c)) / norm)
			if names != nil {
				if _, ok := names[idx]; !ok {
					names[idx] = k
				}
			}
		}
	}
	v.Sparse = make([]Sparse, 0, len(acc))
	for idx, val := range acc {
		v.Sparse = append(v.Sparse, Sparse{Index: idx, Value: val})
	}
	sort.Slice(v.Sparse, func(i, j int) bool { return v.Sparse[i].Index < v.Sparse[j].Index })
	if names != nil {
		v.Names = make([]string, len(v.Sparse))
		for i, s := range v.Sparse {
			v.Names[i] = names[s.Index]
		}
	}
	var msgPart []Sparse
	if len(gMsg.counts) > 0 {
		msgPart = hashGroup(gMsg)
	}
	return v, msgPart
}

func hashGroup(g *group) []Sparse {
	var norm float64
	for _, c := range g.counts {
		l := math.Log1p(float64(c))
		norm += l * l
	}
	norm = math.Sqrt(norm)
	acc := make(map[uint32]float32, len(g.counts))
	for k, c := range g.counts {
		acc[Hash(k)] += float32(math.Log1p(float64(c)) / norm)
	}
	out := make([]Sparse, 0, len(acc))
	for idx, val := range acc {
		out = append(out, Sparse{Index: idx, Value: val})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func hasTestKeyword(l string) bool {
	return strings.Contains(l, "describe(") || strings.Contains(l, "it(") || strings.Contains(l, "test(") || strings.Contains(l, "expect(") ||
		strings.Contains(l, "assert") || strings.Contains(l, "should") || strings.Contains(l, "func test") || strings.Contains(l, "def test_") ||
		strings.Contains(l, "@test") || strings.Contains(l, "#[test]") || strings.Contains(l, "t.run(") || strings.Contains(l, "mock") ||
		strings.Contains(l, "fixture") || strings.Contains(l, "tomatch") || strings.Contains(l, "tobe") || strings.Contains(l, "toequal") ||
		strings.Contains(l, "require.") && strings.Contains(l, "(t,") || strings.Contains(l, "spec") || strings.Contains(l, "stub") ||
		strings.Contains(l, "snapshot") || strings.Contains(l, "beforeeach") || strings.Contains(l, "aftereach") || strings.Contains(l, "@pytest")
}

func hasErrorKeyword(l string) bool {
	return strings.Contains(l, "error") || strings.Contains(l, "err ") || strings.Contains(l, "err.") || strings.Contains(l, "catch") ||
		strings.Contains(l, "throw") || strings.Contains(l, "except") || strings.Contains(l, "panic") || strings.Contains(l, "raise ") ||
		strings.Contains(l, "rescue") || strings.Contains(l, "unwrap") || strings.Contains(l, "?;") || strings.Contains(l, "try") ||
		strings.Contains(l, "fail") || strings.Contains(l, "invalid") || strings.Contains(l, "exception")
}

func hasNullCheck(l string) bool {
	return strings.Contains(l, "!= nil") || strings.Contains(l, "== nil") || strings.Contains(l, "null") || strings.Contains(l, "undefined") ||
		strings.Contains(l, "none") || strings.Contains(l, "?.") || strings.Contains(l, "??") || strings.Contains(l, "is_some") ||
		strings.Contains(l, "is_none") || strings.Contains(l, "optional") || strings.Contains(l, "isset(") || strings.Contains(l, "empty(") ||
		strings.Contains(l, "if (!") || strings.Contains(l, "if !") || strings.Contains(l, "if not ") || strings.Contains(l, "?? ") || strings.Contains(l, "|| ")
}

func hasLogKeyword(l string) bool {
	return strings.Contains(l, "console.") || strings.Contains(l, "log.") || strings.Contains(l, "logger") || strings.Contains(l, "println") ||
		strings.Contains(l, "print(") || strings.Contains(l, "printf") || strings.Contains(l, "debug") || strings.Contains(l, "trace") ||
		strings.Contains(l, "warn") || strings.Contains(l, "logging") || strings.Contains(l, "slog.") || strings.Contains(l, "tracing::")
}

func hasTypeKeyword(l string) bool {
	return strings.Contains(l, "interface ") || strings.Contains(l, "type ") || strings.Contains(l, ": string") || strings.Contains(l, ": number") ||
		strings.Contains(l, "readonly") || strings.Contains(l, "typedef") || strings.Contains(l, "@param") || strings.Contains(l, "@return") ||
		strings.Contains(l, "-> ") || strings.Contains(l, "generic") || strings.Contains(l, "<t>") || strings.Contains(l, "typing") ||
		strings.Contains(l, "struct ") || strings.Contains(l, "enum ") || strings.Contains(l, "class ") || strings.Contains(l, "impl ")
}
