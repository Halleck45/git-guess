package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Halleck45/git-guess/internal/diff"
	"github.com/Halleck45/git-guess/internal/features"
	"github.com/Halleck45/git-guess/internal/gitx"
	"github.com/Halleck45/git-guess/internal/history"
	"github.com/Halleck45/git-guess/internal/model"
)

// checkResult is the verdict for one commit.
type checkResult struct {
	Sha        string  `json:"sha"`
	Subject    string  `json:"subject"`
	Declared   string  `json:"declared,omitempty"`
	Guess      string  `json:"guess"`
	Confidence float64 `json:"confidence"`
	Second     string  `json:"second,omitempty"`
	Status     string  `json:"status"` // ok, missing, disputed
}

const checkUsage = `usage: git guess check [<base>..<head>] [--strict] [--min-confidence 0.75] [--json] [--github]

Checks every commit of the range (default: the current branch against its
upstream, or HEAD~10..HEAD) for a Conventional Commits type, and flags
declared types that the diff contradicts. Exit 1 when a commit has no type;
with --strict, also when a type is disputed.`

// runCheck lints the commits of a range semantically.
func runCheck(args []string, stdout, stderr io.Writer, st style) error {
	strict, asJSON, gh := false, false, false
	minConf := 0.75
	var rng string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--strict":
			strict = true
		case a == "--json":
			asJSON = true
		case a == "--github":
			gh = true
		case a == "--min-confidence":
			if i+1 >= len(args) {
				return fmt.Errorf("--min-confidence needs a value")
			}
			i++
			fmt.Sscanf(args[i], "%g", &minConf)
		case strings.HasPrefix(a, "--min-confidence="):
			fmt.Sscanf(strings.TrimPrefix(a, "--min-confidence="), "%g", &minConf)
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, checkUsage)
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown check flag %s", a)
		default:
			rng = a
		}
	}
	if !gitx.InRepo() {
		return fmt.Errorf("not a git repository")
	}
	if rng == "" {
		rng = defaultRange()
	}
	m, err := model.Default()
	if err != nil {
		return err
	}
	meta, _ := model.DefaultMeta()
	out, err := exec.Command("git", "log", "--no-merges", "--format=%H%x00%s", rng).Output()
	if err != nil {
		return fmt.Errorf("git log %s: %w", rng, err)
	}
	var shas []string
	subjects := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		sha, subj, ok := strings.Cut(line, "\x00")
		if ok {
			shas = append(shas, sha)
			subjects[sha] = subj
		}
	}
	if len(shas) == 0 {
		fmt.Fprintf(stdout, "no commits in %s\n", rng)
		return nil
	}
	// History for adaptation: commits older than the range.
	idx, _ := history.Load(m, len(shas), indexProgress())
	pos := map[string]int{}
	if idx != nil {
		for i, e := range idx.Entries {
			pos[e.Sha] = i
		}
	}
	var results []checkResult
	missing, disputed := 0, 0
	for i := len(shas) - 1; i >= 0; i-- { // oldest first
		sha := shas[i]
		raw, err := gitx.DiffRev(sha)
		if err != nil {
			continue
		}
		d := diff.ParseString(string(raw))
		if len(d.Files) == 0 {
			continue
		}
		v := features.Extract(d, features.Options{})
		z := m.Logits(v)
		p := m.Probabilities(z)
		if idx != nil && meta != nil && len(idx.Entries) >= history.MinEntries {
			paths := make([]string, len(d.Files))
			for k, f := range d.Files {
				paths[k] = f.Path
			}
			ev := idx.Gather(m, v.Sparse, paths, z, -1)
			p = meta.Predict(model.MetaFeatures(p, ev.KNN, ev.Conf, ev.File, ev.Seen, ev.Size, ev.MaxSim))
		}
		c := m.Rank(p)
		r := checkResult{Sha: sha, Subject: subjects[sha], Guess: c[0].Type, Confidence: c[0].P, Second: c[1].Type, Status: "ok"}
		declared, _, ok := gitx.ParseHeader(subjects[sha], m.Classes)
		switch {
		case !ok:
			r.Status = "missing"
			missing++
		default:
			r.Declared = declared
			if declared != c[0].Type && declared != c[1].Type && c[0].P >= minConf {
				r.Status = "disputed"
				disputed++
			}
		}
		results = append(results, r)
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"range": rng, "commits": results, "missing": missing, "disputed": disputed})
	} else {
		for _, r := range results {
			short := r.Sha[:7]
			subj := r.Subject
			if len(subj) > 60 {
				subj = subj[:57] + "..."
			}
			switch r.Status {
			case "ok":
				fmt.Fprintf(stdout, "  %s %s %s\n", st.green("✔"), st.dim(short), subj)
			case "missing":
				fmt.Fprintf(stdout, "  %s %s %s\n      %s %s\n", st.yellow("✘"), st.dim(short), subj,
					st.dim("no type; suggestion:"), st.bold(fmt.Sprintf("%s: %s", r.Guess, r.Subject)))
				if gh {
					fmt.Fprintf(stdout, "::error title=Missing commit type::%s %q has no Conventional Commits type, suggestion: %s\n", short, r.Subject, r.Guess)
				}
			case "disputed":
				fmt.Fprintf(stdout, "  %s %s %s\n      %s %s %s %s %s\n", st.yellow("?"), st.dim(short), subj,
					st.dim("declared"), r.Declared, st.dim("but the diff looks like"), st.bold(r.Guess), st.dim(percent(r.Confidence)))
				if gh {
					fmt.Fprintf(stdout, "::warning title=Disputed commit type::%s declared %s but the diff looks like %s (%s)\n", short, r.Declared, r.Guess, percent(r.Confidence))
				}
			}
		}
		fmt.Fprintf(stdout, "\n%d commit%s checked", len(results), plural(len(results)))
		if missing > 0 {
			fmt.Fprintf(stdout, ", %s", st.yellow(fmt.Sprintf("%d without type", missing)))
		}
		if disputed > 0 {
			fmt.Fprintf(stdout, ", %s", st.yellow(fmt.Sprintf("%d disputed", disputed)))
		}
		fmt.Fprintln(stdout)
	}
	if missing > 0 || strict && disputed > 0 {
		return exitCode(1)
	}
	return nil
}

// defaultRange picks upstream..HEAD when an upstream exists, else HEAD~10..HEAD.
func defaultRange() string {
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}").Output(); err == nil {
		up := strings.TrimSpace(string(out))
		if up != "" {
			if mb, err := exec.Command("git", "merge-base", up, "HEAD").Output(); err == nil {
				return strings.TrimSpace(string(mb)) + "..HEAD"
			}
		}
	}
	return "HEAD~10..HEAD"
}
