package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/Halleck45/conventional/internal/diff"
	"github.com/Halleck45/conventional/internal/features"
	"github.com/Halleck45/conventional/internal/gitx"
	"github.com/Halleck45/conventional/internal/model"
	"github.com/Halleck45/conventional/internal/scope"
)

// Result of a classification.
type Result struct {
	Type       string            `json:"type"`
	Scope      string            `json:"scope,omitempty"`
	Breaking   bool              `json:"breaking,omitempty"`
	Confidence float64           `json:"confidence"`
	Header     string            `json:"header"`
	Candidates []model.Candidate `json:"candidates"`
	Source     string            `json:"source"`
	Files      int               `json:"files"`
	Added      int               `json:"added"`
	Removed    int               `json:"removed"`
	Adapted    bool              `json:"adapted_to_repo"`
	Explain    *Explanation      `json:"explain,omitempty"`
}

// Explanation lists the features that drove the decision.
type Explanation struct {
	Against string               `json:"against"`
	For     []model.Contribution `json:"for"`
	Contra  []model.Contribution `json:"against_features"`
}

type classifyOptions struct {
	message  string
	source   gitx.Source
	rev      string
	stdin    []byte
	noPrior  bool
	noScope  bool
	scope    string
	explain  bool
	top      int
	lambda   float64
	fallback bool
}

// Header formats a conventional commit header.
func (r *Result) formatHeader(subject string) string {
	h := r.Type
	if r.Scope != "" {
		h += "(" + r.Scope + ")"
	}
	if r.Breaking {
		h += "!"
	}
	h += ":"
	if subject != "" {
		h += " " + subject
	}
	return h
}

func classify(opts classifyOptions) (*Result, error) {
	m, err := model.Default()
	if err != nil {
		return nil, err
	}
	var raw []byte
	source := string(opts.source)
	switch {
	case opts.stdin != nil:
		raw = opts.stdin
		source = string(gitx.SourceStdin)
	case opts.rev != "":
		raw, err = gitx.DiffRev(opts.rev)
		if err != nil {
			return nil, err
		}
		source = opts.rev
	default:
		if !gitx.InRepo() {
			return nil, errors.New("not a git repository (pipe a diff on stdin, or run inside a repository)")
		}
		var used gitx.Source
		raw, used, err = gitx.Diff(opts.source, opts.fallback)
		if err != nil {
			return nil, err
		}
		source = string(used)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, gitx.ErrNoChanges
	}
	d, _ := diff.Parse(bytes.NewReader(raw))
	if len(d.Files) == 0 {
		return nil, errors.New("input does not look like a unified diff")
	}
	v := features.Extract(d, features.Options{Message: opts.message, KeepNames: opts.explain})
	logits := m.Logits(v)
	p := m.Probabilities(logits)

	var hist *gitx.History
	adapted := false
	if opts.stdin == nil && gitx.InRepo() {
		hist, _ = gitx.ReadHistory(500, m.Classes)
		if hist != nil && !opts.noPrior && hist.Conventional >= 30 {
			p = m.AdaptPrior(p, hist.Types, opts.lambda)
			adapted = true
		}
	}
	cands := m.Rank(p)
	res := &Result{Type: cands[0].Type, Confidence: cands[0].P, Candidates: cands, Source: source, Files: len(d.Files), Adapted: adapted}
	for _, f := range d.Files {
		res.Added += f.Added
		res.Removed += f.Removed
	}
	if opts.top > 0 && opts.top < len(res.Candidates) {
		res.Candidates = res.Candidates[:opts.top]
	}
	switch {
	case opts.scope != "":
		res.Scope = opts.scope
	case !opts.noScope:
		res.Scope = scope.Guess(d, hist)
	}
	res.Header = res.formatHeader(opts.message)
	if opts.explain {
		other := m.ClassIndex(cands[1].Type)
		pos, neg := m.Explain(v, m.ClassIndex(cands[0].Type), other, 8)
		res.Explain = &Explanation{Against: cands[1].Type, For: pos, Contra: neg}
	}
	return res, nil
}

// hasConventionalPrefix reports whether a subject already carries a type.
func hasConventionalPrefix(subject string, classes []string) bool {
	_, _, ok := gitx.ParseHeader(strings.TrimSpace(subject), classes)
	return ok
}

func percent(p float64) string {
	n := int(p*100 + 0.5)
	if n > 99 {
		n = 99 // never claim certainty
	}
	return fmt.Sprintf("%d%%", n)
}
