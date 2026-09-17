package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Halleck45/git-guess/internal/diff"
	"github.com/Halleck45/git-guess/internal/features"
	"github.com/Halleck45/git-guess/internal/gitmoji"
	"github.com/Halleck45/git-guess/internal/gitx"
	"github.com/Halleck45/git-guess/internal/history"
	"github.com/Halleck45/git-guess/internal/model"
	"github.com/Halleck45/git-guess/internal/scope"
)

// Result of a classification.
type Result struct {
	Type       string              `json:"type"`
	Scope      string              `json:"scope,omitempty"`
	Breaking   bool                `json:"breaking,omitempty"`
	Emoji      string              `json:"emoji,omitempty"`      // gitmoji, when asked for
	EmojiCode  string              `json:"emoji_code,omitempty"` // its :shortcode:
	Confidence float64             `json:"confidence"`
	Header     string              `json:"header"`
	Candidates []model.Candidate   `json:"candidates"`
	Source     string              `json:"source"`
	Files      int                 `json:"files"`
	Added      int                 `json:"added"`
	Removed    int                 `json:"removed"`
	Adapted    bool                `json:"adapted_to_repo"`
	History    int                 `json:"history_commits,omitempty"`
	Nearest    []history.Neighbour `json:"nearest,omitempty"`
	Explain    *Explanation        `json:"explain,omitempty"`

	subject string                         // draft subject the header was built with
	gitmoji string                         // "", "emoji" or "code"
	pick    func(typ string) gitmoji.Emoji // gitmoji of a type for this diff
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
	// gitmoji is "" (conventional header), "emoji" or "code"; the empty
	// value defers to the guess.gitmoji git configuration unless noGitmoji.
	gitmoji   string
	noGitmoji bool
}

// gitmojiMode resolves the output style: the flag wins, then the
// guess.gitmoji configuration (true/emoji, or code for :shortcodes:).
func gitmojiMode(opts classifyOptions) string {
	if opts.gitmoji != "" || opts.noGitmoji {
		return opts.gitmoji
	}
	switch strings.ToLower(gitx.Config("guess.gitmoji")) {
	case "true", "1", "yes", "on", "emoji", "unicode":
		return "emoji"
	case "code", "shortcode", "shortcodes":
		return "code"
	}
	return ""
}

// formatHeader formats the commit header: "type(scope)!: subject", or in
// gitmoji mode "✨ (scope): subject".
func (r *Result) formatHeader(subject string) string {
	if r.gitmoji != "" {
		return gitmoji.Format(r.emoji(), r.Scope, subject, r.gitmoji == "code")
	}
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

// emoji returns the gitmoji of the current type, recomputed after the type
// changed (the user picked the runner-up).
func (r *Result) emoji() gitmoji.Emoji {
	if r.pick == nil {
		return gitmoji.ForType(r.Type)
	}
	return r.pick(r.Type)
}

// setType changes the chosen type and everything derived from it.
func (r *Result) setType(typ string, p float64) {
	r.Type, r.Confidence = typ, p
	if r.gitmoji != "" {
		e := r.emoji()
		r.Emoji, r.EmojiCode = e.Unicode, e.Code
	}
	r.Header = r.formatHeader(r.subject)
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
	histSize := 0
	var nearest []history.Neighbour
	if opts.stdin == nil && gitx.InRepo() {
		hist, _ = gitx.ReadHistory(500, m.Classes)
		if !opts.noPrior {
			p, histSize, nearest = adaptToRepo(m, v, d, logits, p, hist, opts.lambda)
			adapted = histSize > 0
		}
	}
	cands := m.Rank(p)
	res := &Result{Type: cands[0].Type, Confidence: cands[0].P, Candidates: cands, Source: source, Files: len(d.Files), Adapted: adapted, History: histSize}
	if opts.explain {
		res.Nearest = nearest
	}
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
	res.subject = opts.message
	res.gitmoji = gitmojiMode(opts)
	if res.gitmoji != "" {
		res.pick = func(typ string) gitmoji.Emoji { return gitmoji.Pick(typ, res.Breaking, d, opts.message) }
	}
	res.setType(res.Type, res.Confidence)
	if opts.explain && len(cands) > 1 {
		other := m.ClassIndex(cands[1].Type)
		pos, neg := m.Explain(v, m.ClassIndex(cands[0].Type), other, 8)
		res.Explain = &Explanation{Against: cands[1].Type, For: pos, Contra: neg}
	}
	return res, nil
}

// adaptToRepo re-scores the prediction with the repository's own history.
// With enough indexed commits the meta-model combines the global
// probabilities with nearest past diffs, the local confusion pattern and
// per-file history; otherwise it falls back to the marginal prior.
func adaptToRepo(m *model.Model, v *features.Vector, d *diff.Diff, logits, p []float64, hist *gitx.History, lambda float64) ([]float64, int, []history.Neighbour) {
	idx, err := history.Load(m, 0, indexProgress())
	if err == nil && len(idx.Entries) >= history.MinEntries {
		if meta, err := model.DefaultMeta(); err == nil {
			paths := make([]string, 0, len(d.Files))
			for _, f := range d.Files {
				paths = append(paths, f.Path)
			}
			ev := idx.Gather(m, v.Sparse, paths, logits, -1)
			f := model.MetaFeatures(p, ev.KNN, ev.Conf, ev.File, ev.Seen, ev.Size, ev.MaxSim)
			return meta.Predict(f), ev.Size, ev.Nearest
		}
	}
	if hist != nil && hist.Conventional >= 30 {
		return m.AdaptPrior(p, hist.Types, lambda), 0, nil
	}
	return p, 0, nil
}

// indexProgress reports first-run indexing on stderr when it is a terminal.
func indexProgress() history.Progress {
	if !isTerminal(os.Stderr) {
		return nil
	}
	announced := false
	return func(done, total int) {
		if total < 30 {
			return
		}
		if !announced {
			fmt.Fprintf(os.Stderr, "\rindexing %d commits of history (first run only)...", total)
			announced = true
		}
		if done == total {
			fmt.Fprint(os.Stderr, "\r\033[K")
		}
	}
}

// hasConventionalPrefix reports whether a subject already carries a type,
// conventional or gitmoji.
func hasConventionalPrefix(subject string, classes []string) bool {
	subject = strings.TrimSpace(subject)
	_, _, ok := gitx.ParseHeader(subject, classes)
	return ok || gitmoji.HasPrefix(subject)
}

func percent(p float64) string {
	n := int(p*100 + 0.5)
	if n > 99 {
		n = 99 // never claim certainty
	}
	return fmt.Sprintf("%d%%", n)
}
