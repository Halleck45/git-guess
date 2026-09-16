package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type style struct{ color bool }

func (s style) wrap(code, text string) string {
	if !s.color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
func (s style) bold(t string) string   { return s.wrap("1", t) }
func (s style) dim(t string) string    { return s.wrap("2", t) }
func (s style) green(t string) string  { return s.wrap("32", t) }
func (s style) yellow(t string) string { return s.wrap("33", t) }
func (s style) cyan(t string) string   { return s.wrap("36", t) }

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func bar(p float64, width int) string {
	n := int(p*float64(width) + 0.5)
	return strings.Repeat("█", n) + strings.Repeat("░", width-n)
}

// render prints a result. In a terminal it is friendly; when piped it prints
// exactly one line: the header, so it composes with git commit -m "$(...)".
func render(w io.Writer, r *Result, quiet, asJSON, tty bool, st style) {
	switch {
	case asJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(r)
		return
	case quiet:
		fmt.Fprintln(w, r.Type)
		return
	case !tty:
		fmt.Fprintln(w, strings.TrimSuffix(r.Header, ":"))
		return
	}
	head := r.Header
	if strings.HasSuffix(head, ":") {
		head = strings.TrimSuffix(head, ":")
	}
	conf := st.green(percent(r.Confidence))
	mark := st.green("✔")
	if r.Confidence < 0.5 {
		conf = st.yellow(percent(r.Confidence))
		mark = st.yellow("?")
	}
	fmt.Fprintf(w, "%s %s  %s %s\n", mark, st.bold(head), st.dim(bar(r.Confidence, 10)), conf)
	if len(r.Candidates) > 1 {
		var alts []string
		for _, c := range r.Candidates[1:] {
			if c.P < 0.01 || c.P < 0.03 && len(alts) >= 1 {
				break
			}
			alts = append(alts, fmt.Sprintf("%s %s", c.Type, st.dim(percent(c.P))))
		}
		if len(alts) > 0 {
			fmt.Fprintf(w, "  %s %s\n", st.dim("also:"), strings.Join(alts, st.dim(" · ")))
		}
	}
	if len(r.Nearest) > 0 {
		fmt.Fprintf(w, "\n  %s\n", st.dim("closest past commits of this repo"))
		for i, n := range r.Nearest {
			if i >= 5 {
				break
			}
			fmt.Fprintf(w, "    %s %-9s %s\n", st.dim(n.Sha[:7]), n.Type, st.dim(fmt.Sprintf("%.0f%% similar", n.Sim*100)))
		}
	}
	if r.Explain != nil {
		fmt.Fprintf(w, "\n  %s %s %s %s\n", st.dim("why"), st.bold(r.Type), st.dim("rather than"), st.bold(r.Explain.Against))
		for _, c := range r.Explain.For {
			fmt.Fprintf(w, "    %s %-40s %+.2f\n", st.green("+"), describe(c.Feature), c.Weight)
		}
		for _, c := range r.Explain.Contra {
			fmt.Fprintf(w, "    %s %-40s %+.2f\n", st.yellow("−"), describe(c.Feature), c.Weight)
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// describe turns an internal feature name into something readable.
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
	case "s":
		return "file " + val
	case "cs", "es", "ls", "ce", "fe":
		return "file " + strings.ReplaceAll(val, "_", " ")
	case "a":
		return "added token " + val
	case "r":
		return "removed token " + val
	case "x":
		return "new identifier " + val
	case "y":
		return "dropped identifier " + val
	case "ab":
		return "added \"" + strings.ReplaceAll(val, "_", " ") + "\""
	case "rb":
		return "removed \"" + strings.ReplaceAll(val, "_", " ") + "\""
	case "A1", "AL":
		return "added line starting with " + strings.TrimPrefix(val[strings.LastIndex(val, ":")+1:], ":")
	case "R1", "RL":
		return "removed line starting with " + val[strings.LastIndex(val, ":")+1:]
	case "h", "hs":
		return "in function " + val
	case "m", "m1", "m2":
		return "message word " + strings.ReplaceAll(val, "_", " ")
	}
	return f
}
