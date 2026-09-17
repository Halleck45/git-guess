package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Halleck45/git-guess/internal/gitx"
	"github.com/Halleck45/git-guess/internal/model"
)

const hookMarker = "# managed by git-guess"

const hookScript = `#!/bin/sh
` + hookMarker + ` — https://github.com/Halleck45/git-guess
command -v git-guess >/dev/null 2>&1 || exit 0
exec git-guess hook run "$@"
`

func runHook(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: git guess hook install|uninstall|run")
	}
	switch args[0] {
	case "install":
		return hookInstall(args[1:], stdout)
	case "uninstall":
		return hookUninstall(stdout)
	case "run":
		return hookRun(args[1:], stderr)
	}
	return fmt.Errorf("unknown hook command %q", args[0])
}

func hookPath() (string, bool, error) {
	if !gitx.InRepo() {
		return "", false, fmt.Errorf("not a git repository")
	}
	dir, custom, err := gitx.HooksDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(dir, "prepare-commit-msg"), custom, nil
}

func hookInstall(args []string, stdout io.Writer) error {
	force, withGitmoji := false, false
	for _, a := range args {
		switch a {
		case "--force":
			force = true
		case "--gitmoji":
			withGitmoji = true
		default:
			return fmt.Errorf("unknown hook install flag %s (--force, --gitmoji)", a)
		}
	}
	p, custom, err := hookPath()
	if err != nil {
		return err
	}
	if withGitmoji {
		if err := gitx.SetConfig("guess.gitmoji", "true"); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "set guess.gitmoji=true in this repository: headers will be gitmoji (git config --unset guess.gitmoji to go back)")
	}
	if b, err := os.ReadFile(p); err == nil {
		if strings.Contains(string(b), hookMarker) {
			fmt.Fprintf(stdout, "hook already installed at %s\n", p)
			return nil
		}
		if !force {
			return fmt.Errorf("%s already exists and is not ours.\nAdd this line to it:\n\n    git-guess hook run \"$@\"\n\nor re-run with --force to replace it", p)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(hookScript), 0o755); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "installed %s\n", p)
	if custom {
		fmt.Fprintln(stdout, "note: core.hooksPath is set, the hook was written there")
	}
	example := "feat: add login"
	if gitmojiMode(classifyOptions{}) != "" {
		example = "✨ add login"
	}
	fmt.Fprintf(stdout, "git commit -m \"add login\" now becomes %q (uninstall with: git guess hook uninstall)\n", example)
	return nil
}

func hookUninstall(stdout io.Writer) error {
	p, _, err := hookPath()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Fprintln(stdout, "no hook installed")
		return nil
	}
	if !strings.Contains(string(b), hookMarker) {
		return fmt.Errorf("%s is not managed by git-guess, leaving it alone", p)
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed %s\n", p)
	return nil
}

// hookRun implements prepare-commit-msg: $1 = message file, $2 = source
// (empty, message, template, merge, squash, commit), $3 = sha for amend.
func hookRun(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		return nil
	}
	file := args[0]
	source := ""
	if len(args) > 1 {
		source = args[1]
	}
	switch source {
	case "merge", "squash", "commit":
		return nil // keep git's own message
	}
	m, err := model.Default()
	if err != nil {
		return nil // never block a commit
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	if len(args) > 2 && args[2] != "" {
		return nil // amend: the staged delta is not the whole change
	}
	if os.Getenv("GIT_GUESS_HOOK") == "0" {
		return nil
	}
	content := string(b)
	ch, strip := gitx.CommentChar()
	var first string
	firstIdx := 0
	if source == "message" {
		// git keeps every line of a -m message (no comment stripping), so
		// the subject is the raw first line.
		first = strings.TrimSpace(strings.SplitN(content, "\n", 2)[0])
	} else {
		first, firstIdx = firstContentLine(content, ch)
	}
	if hasConventionalPrefix(first, m.Classes) {
		return nil
	}
	for _, p := range []string{"fixup! ", "squash! ", "amend! ", "Revert \"", "Reapply \""} {
		if strings.HasPrefix(first, p) {
			return nil // git generated this subject on purpose
		}
	}
	res, err := classify(classifyOptions{source: gitx.SourceStaged, fallback: false, top: 2, lambda: 0.7, message: first})
	if err != nil {
		return nil
	}
	maybeAsk(res, os.Stderr)
	var out string
	if first == "" {
		// Interactive commit: prefill "type(scope): " on the first line.
		out = res.formatHeader("") + " \n" + strings.TrimPrefix(content, "\n")
		if strip {
			// Add a comment so the user sees the alternatives.
			out += fmt.Sprintf("%s\n%s git guess: %s (%s)", ch, ch, res.Type, percent(res.Confidence))
			if len(res.Candidates) > 1 {
				out += fmt.Sprintf(", or %s (%s)", res.Candidates[1].Type, percent(res.Candidates[1].P))
			}
			out += "\n"
		}
	} else {
		lines := strings.Split(content, "\n")
		lines[firstIdx] = res.formatHeader(first)
		out = strings.Join(lines, "\n")
		fmt.Fprintln(stderr, "git guess:", res.formatHeader(first))
	}
	if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
		fmt.Fprintln(stderr, "git-guess: could not update the message:", err)
	}
	return nil // never block a commit
}

// firstContentLine returns the first non-comment line and its index.
func firstContentLine(content, commentChar string) (string, int) {
	sc := bufio.NewScanner(strings.NewReader(content))
	i := 0
	for sc.Scan() {
		l := sc.Text()
		if !strings.HasPrefix(l, commentChar) {
			return strings.TrimSpace(l), i
		}
		i++
	}
	return "", 0
}
