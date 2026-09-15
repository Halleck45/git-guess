// Package gitx wraps the git invocations the CLI needs: obtaining the diff to
// classify and reading the repository history to learn local conventions.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Source describes where a diff came from.
type Source string

const (
	SourceStdin    Source = "stdin"
	SourceStaged   Source = "staged"
	SourceUnstaged Source = "unstaged"
	SourceAll      Source = "all"
	SourceRev      Source = "rev"
)

// ErrNoChanges is returned when there is nothing to classify.
var ErrNoChanges = errors.New("no changes found")

func run(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out, nil
}

// InRepo reports whether the working directory is inside a git repository.
func InRepo() bool {
	_, err := run("rev-parse", "--git-dir")
	return err == nil
}

// diffArgs common to every diff invocation: no color, no external diff, with
// rename detection and a stable output format.
var diffArgs = []string{"-c", "diff.noprefix=false", "-c", "core.quotepath=false", "diff", "--no-color", "--no-ext-diff", "-M", "--unified=3"}

// Diff returns the unified diff for the requested source. With SourceStaged
// it falls back to the working tree when the index is clean, and reports the
// source actually used.
func Diff(src Source, allowFallback bool) ([]byte, Source, error) {
	switch src {
	case SourceStaged:
		out, err := run(append(diffArgs, "--cached")...)
		if err != nil {
			return nil, src, err
		}
		if len(bytes.TrimSpace(out)) > 0 {
			return out, SourceStaged, nil
		}
		if !allowFallback {
			return nil, src, ErrNoChanges
		}
		out, err = run(diffArgs...)
		if err != nil {
			return nil, src, err
		}
		if len(bytes.TrimSpace(out)) == 0 {
			return nil, src, ErrNoChanges
		}
		return out, SourceUnstaged, nil
	case SourceUnstaged:
		out, err := run(diffArgs...)
		if err != nil {
			return nil, src, err
		}
		if len(bytes.TrimSpace(out)) == 0 {
			return nil, src, ErrNoChanges
		}
		return out, SourceUnstaged, nil
	case SourceAll:
		out, err := run(append(diffArgs, "HEAD")...)
		if err != nil {
			// unborn branch: everything staged is the diff
			out, err = run(append(diffArgs, "--cached")...)
			if err != nil {
				return nil, src, err
			}
		}
		if len(bytes.TrimSpace(out)) == 0 {
			return nil, src, ErrNoChanges
		}
		return out, SourceAll, nil
	}
	return nil, src, fmt.Errorf("unknown source %q", src)
}

// DiffRev returns the diff of a revision (single commit) or a range (a..b).
func DiffRev(rev string) ([]byte, error) {
	if strings.Contains(rev, "..") {
		return run(append(diffArgs, rev)...)
	}
	return run("-c", "core.quotepath=false", "show", "--no-color", "--no-ext-diff", "-M", "--unified=3", "--format=", rev)
}

var ccRe = regexp.MustCompile(`^([A-Za-z]+)(?:\(([^)]*)\))?!?:\s`)

var aliases = map[string]string{"tests": "test", "feature": "feat", "bugfix": "fix", "doc": "docs", "bug": "fix"}

// ParseHeader extracts the conventional type and scope from a commit subject.
func ParseHeader(subject string, known []string) (typ, scope string, ok bool) {
	m := ccRe.FindStringSubmatch(subject)
	if m == nil {
		return "", "", false
	}
	t := strings.ToLower(m[1])
	if a, ok := aliases[t]; ok {
		t = a
	}
	for _, k := range known {
		if k == t {
			return t, strings.TrimSpace(m[2]), true
		}
	}
	return "", "", false
}

// History summarizes the recent commit subjects of the repository.
type History struct {
	Commits      int            // commits inspected
	Conventional int            // commits with a recognized type
	Types        map[string]int // type -> count
	Scopes       map[string]int // scope -> count (only among conventional commits)
	WithScope    int            // conventional commits carrying a scope
}

// ReadHistory inspects the last n commit subjects.
func ReadHistory(n int, known []string) (*History, error) {
	out, err := run("log", "--no-merges", "--format=%s", fmt.Sprintf("-n%d", n))
	if err != nil {
		return nil, err
	}
	h := &History{Types: map[string]int{}, Scopes: map[string]int{}}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		h.Commits++
		t, s, ok := ParseHeader(line, known)
		if !ok {
			continue
		}
		h.Conventional++
		h.Types[t]++
		if s != "" {
			h.WithScope++
			h.Scopes[s]++
		}
	}
	return h, nil
}

// HooksDir returns the directory where hooks live, honoring core.hooksPath.
func HooksDir() (string, bool, error) {
	if out, err := run("config", "--get", "core.hooksPath"); err == nil && len(bytes.TrimSpace(out)) > 0 {
		return strings.TrimSpace(string(out)), true, nil
	}
	out, err := run("rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(out)), false, nil
}
