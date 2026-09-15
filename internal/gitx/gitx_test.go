package gitx

import "testing"

func TestParseHeader(t *testing.T) {
	known := []string{"feat", "fix", "docs", "chore", "test"}
	cases := []struct {
		in         string
		typ, scope string
		ok         bool
	}{
		{"feat(auth): add login", "feat", "auth", true},
		{"Fix: typo", "fix", "", true},
		{"tests: add case", "test", "", true},
		{"refactor!: drop", "", "", false}, // refactor not in known list here
		{"chore(deps)!: bump", "chore", "deps", true},
		{"Merge branch 'x'", "", "", false},
		{"feat:no space", "", "", false},
		{"[docs] x", "", "", false},
	}
	for _, c := range cases {
		typ, scope, ok := ParseHeader(c.in, known)
		if typ != c.typ || scope != c.scope || ok != c.ok {
			t.Errorf("%q: got (%q,%q,%v) want (%q,%q,%v)", c.in, typ, scope, ok, c.typ, c.scope, c.ok)
		}
	}
}

func TestParseHeaderBare(t *testing.T) {
	if typ, _, ok := ParseHeader("docs:", []string{"docs"}); !ok || typ != "docs" {
		t.Errorf("bare header not recognized")
	}
	if _, _, ok := ParseHeader("docs:x", []string{"docs"}); ok {
		t.Errorf("docs:x should not match")
	}
}
