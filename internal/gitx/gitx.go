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
		if strings.Contains(msg, "not a git repository") {
			return nil, errors.New("not a git repository (pipe a diff on stdin, or run inside a repository)")
		}
		return nil, fmt.Errorf("git %s: %s", subcommand(args), strings.TrimPrefix(msg, "fatal: "))
	}
	return out, nil
}

// subcommand returns the git subcommand from args, skipping leading -c key=value pairs.
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

// InRepo reports whether the working directory is inside a git repository.
func InRepo() bool {
	_, err := run("rev-parse", "--git-dir")
	return err == nil
}

// diffArgs common to every diff invocation: no color, no external diff, with
// rename detection and a stable output format.
var gitCfg = []string{"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false", "-c", "core.quotepath=false"}

var diffArgs = append(append([]string{}, gitCfg...), "diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--unified=3")

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
		out, err := run(append(diffArgs, "HEAD", "--")...)
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
		return run(append(diffArgs, rev, "--")...)
	}
	show := append(append([]string{}, gitCfg...), "show", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--unified=3", "--format=")
	// Merge commits: diff against the first parent rather than a combined diff.
	out, err := run(append(show, "--diff-merges=first-parent", rev, "--")...)
	if err != nil { // older git without --diff-merges
		out, err = run(append(show, rev, "--")...)
	}
	return out, err
}

var ccRe = regexp.MustCompile(`^([A-Za-z]+)(?:\(([^)]*)\))?!?:(?:\s|$)`)

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
	return ReadHistoryRange(0, n, known)
}

// ReadHistoryRange inspects n commit subjects, skipping the most recent
// `skip` ones.
func ReadHistoryRange(skip, n int, known []string) (*History, error) {
	out, err := run("log", "--no-merges", "--format=%s", fmt.Sprintf("--skip=%d", skip), fmt.Sprintf("-n%d", n))
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

// HooksDir returns the directory where hooks live. git rev-parse already
// honors core.hooksPath (and resolves it against the work tree root); the
// boolean only reports whether a custom path is configured.
func HooksDir() (string, bool, error) {
	out, err := run("rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", false, err
	}
	custom := false
	if c, err := run("config", "--get", "core.hooksPath"); err == nil && len(bytes.TrimSpace(c)) > 0 {
		custom = true
	}
	return strings.TrimSpace(string(out)), custom, nil
}

// CommentChar returns the character git treats as a comment in commit
// messages, and whether git will strip comment lines at all (commit.cleanup).
func CommentChar() (string, bool) {
	ch := "#"
	if out, err := run("config", "--get", "core.commentChar"); err == nil {
		v := strings.TrimSpace(string(out))
		if v != "" && v != "auto" {
			ch = v
		}
	}
	strip := true
	if out, err := run("config", "--get", "commit.cleanup"); err == nil {
		switch strings.TrimSpace(string(out)) {
		case "whitespace", "verbatim", "scissors":
			strip = false
		}
	}
	return ch, strip
}
