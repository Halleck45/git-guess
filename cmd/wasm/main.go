//go:build js && wasm

// Command wasm exposes the classifier to the browser playground. It has no
// git and no history: it is the global model, exactly what a diff piped on
// stdin gets.
package main

import (
	"encoding/json"
	"strings"
	"syscall/js"

	"github.com/Halleck45/git-guess/internal/diff"
	"github.com/Halleck45/git-guess/internal/features"
	"github.com/Halleck45/git-guess/internal/model"
	"github.com/Halleck45/git-guess/internal/scope"
)

type contribution struct {
	Feature string  `json:"feature"`
	Weight  float64 `json:"weight"`
}

type result struct {
	Type       string            `json:"type"`
	Scope      string            `json:"scope,omitempty"`
	Header     string            `json:"header"`
	Confidence float64           `json:"confidence"`
	Candidates []model.Candidate `json:"candidates"`
	Files      int               `json:"files"`
	Added      int               `json:"added"`
	Removed    int               `json:"removed"`
	Against    string            `json:"against"`
	For        []contribution    `json:"for"`
	Contra     []contribution    `json:"contra"`
	Error      string            `json:"error,omitempty"`
}

func guess(text string) result {
	m, err := model.Default()
	if err != nil {
		return result{Error: err.Error()}
	}
	d := diff.ParseString(text)
	if len(d.Files) == 0 {
		return result{Error: "that does not look like a unified diff (git diff, git show, or a .patch file)"}
	}
	v := features.Extract(d, features.Options{KeepNames: true})
	cands := m.Predict(v)
	r := result{Type: cands[0].Type, Confidence: cands[0].P, Candidates: cands[:min(4, len(cands))], Files: len(d.Files)}
	for _, f := range d.Files {
		r.Added += f.Added
		r.Removed += f.Removed
	}
	r.Scope = scope.Guess(d, nil)
	r.Header = r.Type
	if r.Scope != "" {
		r.Header += "(" + r.Scope + ")"
	}
	r.Header += ":"
	if len(cands) > 1 {
		r.Against = cands[1].Type
		pos, neg := m.Explain(v, m.ClassIndex(cands[0].Type), m.ClassIndex(cands[1].Type), 6)
		for _, c := range pos {
			r.For = append(r.For, contribution{describe(c.Feature), c.Weight})
		}
		for _, c := range neg {
			r.Contra = append(r.Contra, contribution{describe(c.Feature), c.Weight})
		}
	}
	return r
}

// describe mirrors the CLI's feature naming.
func describe(f string) string {
	if f == "" {
		return "(hashed feature)"
	}
	if strings.HasPrefix(f, "lines_in_") {
		return "lines changed in " + strings.TrimPrefix(f, "lines_in_") + " files"
	}
	i := strings.IndexByte(f, ':')
	if i < 0 {
		return strings.ReplaceAll(f, "_", " ")
	}
	prefix, val := f[:i], f[i+1:]
	switch prefix {
	case "only":
		return "every file is " + val
	case "files":
		return "share of " + val + " files"
	case "p", "p0", "pp", "pl":
		return "directory " + val
	case "f":
		return "file " + val
	case "fw":
		return "file name word " + val
	case "e":
		return "extension ." + val
	case "l":
		return "language " + val
	case "c":
		return "file kind " + val
	case "s", "cs", "es", "ls", "ce", "fe":
		return "file " + strings.ReplaceAll(val, "_", " ")
	case "a":
		return "added token " + val
	case "r":
		return "removed token " + val
	case "x":
		return "new identifier " + val
	case "y":
		return "dropped identifier " + val
	case "A1", "AL":
		return "added line starting with " + val[strings.LastIndex(val, ":")+1:]
	case "R1", "RL":
		return "removed line starting with " + val[strings.LastIndex(val, ":")+1:]
	case "h", "hs":
		return "in function " + val
	}
	return f
}

func main() {
	js.Global().Set("gitGuess", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return `{"error":"no input"}`
		}
		b, _ := json.Marshal(guess(args[0].String()))
		return string(b)
	}))
	select {}
}
