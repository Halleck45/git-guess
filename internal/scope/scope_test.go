package scope

import (
	"testing"

	"github.com/Halleck45/conventional/internal/diff"
	"github.com/Halleck45/conventional/internal/gitx"
)

func mk(paths ...string) *diff.Diff {
	d := &diff.Diff{}
	for _, p := range paths {
		d.Files = append(d.Files, diff.File{Path: p})
	}
	return d
}

func TestGuess(t *testing.T) {
	hist := &gitx.History{Conventional: 100, WithScope: 80, Scopes: map[string]int{"core": 30, "Router": 10}}
	noScopes := &gitx.History{Conventional: 100, WithScope: 2, Scopes: map[string]int{}}
	cases := []struct {
		d    *diff.Diff
		h    *gitx.History
		want string
	}{
		{mk("packages/core/src/a.ts", "packages/core/src/b.ts"), nil, "core"},
		{mk("packages/core/src/a.ts", "packages/ui/src/b.ts"), nil, ""},
		{mk("src/router/a.ts", "src/router/b.ts"), hist, "Router"},
		{mk("src/router/a.ts"), nil, ""},
		{mk("src/router/a.ts"), hist, "Router"},
		{mk("packages/core/a.ts"), noScopes, ""},
		{mk("README.md"), hist, ""},
		{mk("cmd/server/main.go", "cmd/server/x.go", "docs/a.md"), nil, "server"},
	}
	for _, c := range cases {
		if got := Guess(c.d, c.h); got != c.want {
			t.Errorf("%v: got %q want %q", c.d.Files, got, c.want)
		}
	}
}
