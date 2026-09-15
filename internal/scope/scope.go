// Package scope guesses a conventional commit scope from the touched paths
// and the repository's own habits.
package scope

import (
	"path"
	"sort"
	"strings"

	"github.com/Halleck45/conventional/internal/diff"
	"github.com/Halleck45/conventional/internal/gitx"
)

// containerDirs are directories whose children are natural scopes.
var containerDirs = map[string]bool{"packages": true, "apps": true, "libs": true, "lib": true, "crates": true, "modules": true, "services": true,
	"plugins": true, "internal": true, "cmd": true, "pkg": true, "src": true, "components": true, "projects": true, "extensions": true, "integrations": true,
	"providers": true, "adapters": true, "connectors": true, "workspaces": true, "app": true, "api": true, "core": true, "features": true, "domain": true}

// skipDirs are never scopes.
var skipDirs = map[string]bool{"test": true, "tests": true, "__tests__": true, "spec": true, "docs": true, "doc": true, "src": true, "lib": true,
	"internal": true, "pkg": true, "cmd": true, "main": true, "java": true, "kotlin": true, "resources": true, "utils": true, "util": true, "common": true,
	"shared": true, "index": true, "js": true, "ts": true, "go": true, "python": true, "scripts": true, "config": true, "assets": true, "static": true,
	"public": true, "vendor": true, "node_modules": true, "dist": true, "build": true, "types": true, "helpers": true, "": true}

// candidate returns the most plausible scope for one path, and whether it
// comes from a strong signal (a monorepo container directory).
func candidate(p string) (string, bool) {
	segs := strings.Split(strings.ToLower(path.Dir(p)), "/")
	if len(segs) == 1 && segs[0] == "." {
		return "", false
	}
	// strong: <container>/<name>/...
	for i := 0; i+1 < len(segs); i++ {
		if containerDirs[segs[i]] && !skipDirs[segs[i+1]] && !strings.HasPrefix(segs[i+1], ".") {
			return strings.TrimPrefix(segs[i+1], "@"), containerDirs[segs[i]] && segs[i] != "src" && segs[i] != "components" && segs[i] != "core" && segs[i] != "app" && segs[i] != "api"
		}
	}
	// weak: first directory that is not generic
	for _, s := range segs {
		if !skipDirs[s] && !strings.HasPrefix(s, ".") {
			return s, false
		}
	}
	return "", false
}

// Guess returns a scope or "" when no scope should be added.
func Guess(d *diff.Diff, h *gitx.History) string {
	if len(d.Files) == 0 {
		return ""
	}
	counts := map[string]int{}
	strong := map[string]bool{}
	for _, f := range d.Files {
		c, s := candidate(f.Path)
		if c == "" {
			continue
		}
		counts[c]++
		if s {
			strong[c] = true
		}
	}
	if len(counts) == 0 {
		return ""
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v || list[i].v == list[j].v && list[i].k < list[j].k })
	best := list[0]
	// The candidate must cover a clear majority of the files.
	if best.v*2 <= len(d.Files) && len(d.Files) > 1 {
		return ""
	}
	if h != nil && h.Conventional >= 20 {
		rate := float64(h.WithScope) / float64(h.Conventional)
		// Prefer the repository's own spelling when it matches.
		for s := range h.Scopes {
			if strings.EqualFold(s, best.k) || strings.EqualFold(strings.TrimPrefix(s, "@"), best.k) {
				return s
			}
		}
		if rate < 0.15 {
			return ""
		}
		if rate >= 0.5 && (strong[best.k] || best.v == len(d.Files)) {
			return best.k
		}
		return ""
	}
	if strong[best.k] {
		return best.k
	}
	return ""
}
