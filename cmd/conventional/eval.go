package main

import (
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/Halleck45/conventional/internal/diff"
	"github.com/Halleck45/conventional/internal/features"
	"github.com/Halleck45/conventional/internal/gitx"
	"github.com/Halleck45/conventional/internal/model"
)

// runEval replays the last n conventional commits of the current repository
// and compares the guess with the type the author chose. The repository
// prior is computed from commits older than the evaluated window, so the
// numbers are what a user would have seen at the time.
func runEval(args []string, stdout io.Writer, st style) error {
	n := 200
	withMsg := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n", "--last":
			if i+1 >= len(args) {
				return fmt.Errorf("-n needs a value")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil {
				return err
			}
			n = v
		case "--with-message":
			withMsg = true
		default:
			return fmt.Errorf("unknown eval flag %s", args[i])
		}
	}
	if !gitx.InRepo() {
		return fmt.Errorf("not a git repository")
	}
	m, err := model.Default()
	if err != nil {
		return err
	}
	out, err := exec.Command("git", "log", "--no-merges", "--format=%H%x00%s", fmt.Sprintf("-n%d", n)).Output()
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	hist, _ := gitx.ReadHistoryRange(n, 500, m.Classes)
	usePrior := hist != nil && hist.Conventional >= 30
	type row struct{ sha, subject, want string }
	var rows []row
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		sha, subject, ok := strings.Cut(line, "\x00")
		if !ok {
			continue
		}
		typ, _, ok := gitx.ParseHeader(subject, m.Classes)
		if !ok {
			continue
		}
		rows = append(rows, row{sha, subject, typ})
	}
	if len(rows) == 0 {
		return fmt.Errorf("no conventional commits among the last %d", n)
	}
	correct, top2, total := 0, 0, 0
	perType := map[string][2]int{}
	confusion := map[string]map[string]int{}
	for _, r := range rows {
		raw, err := gitx.DiffRev(r.sha)
		if err != nil {
			continue
		}
		d := diff.ParseString(string(raw))
		if len(d.Files) == 0 {
			continue
		}
		msg := ""
		if withMsg {
			if _, rest, ok := strings.Cut(r.subject, ": "); ok {
				msg = rest
			}
		}
		v := features.Extract(d, features.Options{Message: msg})
		p := m.Probabilities(m.Logits(v))
		if usePrior {
			p = m.AdaptPrior(p, hist.Types, 0.7)
		}
		c := m.Rank(p)
		total++
		pt := perType[r.want]
		pt[1]++
		if c[0].Type == r.want {
			correct++
			pt[0]++
		}
		if c[0].Type == r.want || c[1].Type == r.want {
			top2++
		}
		perType[r.want] = pt
		if confusion[r.want] == nil {
			confusion[r.want] = map[string]int{}
		}
		confusion[r.want][c[0].Type]++
	}
	if total == 0 {
		return fmt.Errorf("nothing to evaluate")
	}
	fmt.Fprintf(stdout, "%s %d commits of this repository\n", st.bold("replayed"), total)
	fmt.Fprintf(stdout, "  top-1 %s   top-2 %s", st.bold(percent(float64(correct)/float64(total))), st.bold(percent(float64(top2)/float64(total))))
	if usePrior {
		fmt.Fprint(stdout, st.dim("   (using this repo's history)"))
	}
	fmt.Fprintln(stdout)
	types := make([]string, 0, len(perType))
	for t := range perType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return perType[types[i]][1] > perType[types[j]][1] })
	fmt.Fprintln(stdout)
	for _, t := range types {
		pt := perType[t]
		var conf []string
		var others []string
		for o := range confusion[t] {
			if o != t {
				others = append(others, o)
			}
		}
		sort.Slice(others, func(i, j int) bool { return confusion[t][others[i]] > confusion[t][others[j]] })
		for i, o := range others {
			if i >= 2 {
				break
			}
			conf = append(conf, fmt.Sprintf("%s %d", o, confusion[t][o]))
		}
		line := fmt.Sprintf("  %-9s %4d/%-4d %s", t, pt[0], pt[1], st.dim(bar(float64(pt[0])/float64(pt[1]), 10)))
		if len(conf) > 0 {
			line += st.dim("  guessed as " + strings.Join(conf, ", "))
		}
		fmt.Fprintln(stdout, line)
	}
	return nil
}
