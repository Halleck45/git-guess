package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/Halleck45/conventional/internal/gitx"
	"github.com/Halleck45/conventional/internal/history"
	"github.com/Halleck45/conventional/internal/model"
)

// runEval replays the last n conventional commits of the current repository
// and compares the guess with the type the author chose. Each commit is
// scored with only the commits older than it, exactly as at the time.
func runEval(args []string, stdout io.Writer, st style) error {
	n := 200
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
	meta, err := model.DefaultMeta()
	if err != nil {
		return err
	}
	idx, err := history.Load(m, 0, indexProgress())
	if err != nil {
		return err
	}
	total := len(idx.Entries)
	if total == 0 {
		return fmt.Errorf("no conventional commits found in the last %d commits", history.Window*2)
	}
	start := max(0, total-n)
	correct, top2, scored := 0, 0, 0
	globalCorrect := 0
	perType := map[string][2]int{}
	confusion := map[string]map[string]int{}
	for j := start; j < total; j++ {
		e := idx.Entries[j]
		z := make([]float64, len(e.Logits))
		for i, x := range e.Logits {
			z[i] = float64(x)
		}
		p := m.Probabilities(z)
		if p[argmaxF(p)] > 0 && m.Classes[argmaxF(p)] == m.Classes[e.Type] {
			globalCorrect++
		}
		if j >= history.MinEntries {
			ev := idx.Gather(m, e.Vec, e.Paths, z, j)
			p = meta.Predict(model.MetaFeatures(p, ev.KNN, ev.Conf, ev.File, ev.Seen, ev.Size, ev.MaxSim))
		}
		c := m.Rank(p)
		want := m.Classes[e.Type]
		scored++
		pt := perType[want]
		pt[1]++
		if c[0].Type == want {
			correct++
			pt[0]++
		}
		if c[0].Type == want || c[1].Type == want {
			top2++
		}
		perType[want] = pt
		if confusion[want] == nil {
			confusion[want] = map[string]int{}
		}
		confusion[want][c[0].Type]++
	}
	if scored == 0 {
		return fmt.Errorf("nothing to evaluate")
	}
	fmt.Fprintf(stdout, "%s %d commits of this repository, each scored with only the %s\n", st.bold("replayed"), scored, st.dim("commits before it"))
	fmt.Fprintf(stdout, "  top-1 %s   top-2 %s   %s\n", st.bold(percent(float64(correct)/float64(scored))), st.bold(percent(float64(top2)/float64(scored))),
		st.dim(fmt.Sprintf("(global model alone: %s)", percent(float64(globalCorrect)/float64(scored)))))
	types := make([]string, 0, len(perType))
	for t := range perType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return perType[types[i]][1] > perType[types[j]][1] })
	fmt.Fprintln(stdout)
	for _, t := range types {
		pt := perType[t]
		var others []string
		for o := range confusion[t] {
			if o != t {
				others = append(others, o)
			}
		}
		sort.Slice(others, func(i, j int) bool { return confusion[t][others[i]] > confusion[t][others[j]] })
		var conf []string
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

func argmaxF(p []float64) int {
	b := 0
	for i, x := range p {
		if x > p[b] {
			b = i
		}
	}
	return b
}
