package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Halleck45/conventional/internal/gitx"
	"github.com/Halleck45/conventional/internal/model"
)

// runCommit wraps `git commit`. A -m subject gets the header prefilled;
// without -m the editor opens on a template starting with the header.
func runCommit(args []string, opts classifyOptions, stdin *os.File, stdout, stderr io.Writer) error {
	var subject string
	var rest []string
	hasAll := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-m" || a == "--message":
			if i+1 >= len(args) {
				return fmt.Errorf("-m needs a value")
			}
			i++
			subject = args[i]
		case strings.HasPrefix(a, "--message="):
			subject = strings.TrimPrefix(a, "--message=")
		case strings.HasPrefix(a, "-m") && len(a) > 2:
			subject = a[2:]
		case a == "-a" || a == "--all" || strings.HasPrefix(a, "-a") && !strings.HasPrefix(a, "--") && strings.Contains(a, "a"):
			hasAll = true
			rest = append(rest, a)
		default:
			rest = append(rest, a)
		}
	}
	m, err := model.Default()
	if err != nil {
		return err
	}
	if subject != "" && hasConventionalPrefix(subject, m.Classes) {
		// Already conventional: nothing to guess.
		return execGit(append([]string{"commit", "-m", subject}, rest...), stdin, stdout, stderr)
	}
	opts.source = gitx.SourceStaged
	opts.fallback = false
	if hasAll {
		opts.source = gitx.SourceAll
	}
	res, err := classify(opts)
	if err != nil {
		return err
	}
	if subject != "" {
		header := res.formatHeader(subject)
		fmt.Fprintln(stderr, header)
		return execGit(append([]string{"commit", "-m", header}, rest...), stdin, stdout, stderr)
	}
	tmp, err := os.CreateTemp("", "conventional-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	fmt.Fprintf(tmp, "%s \n# conventional guessed %s (%s)", res.formatHeader(""), res.Type, percent(res.Confidence))
	if len(res.Candidates) > 1 {
		fmt.Fprintf(tmp, ", also %s (%s)", res.Candidates[1].Type, percent(res.Candidates[1].P))
	}
	fmt.Fprintln(tmp)
	tmp.Close()
	return execGit(append([]string{"commit", "--template", filepath.ToSlash(tmp.Name())}, rest...), stdin, stdout, stderr)
}

func execGit(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	cmd := exec.Command("git", args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return exitCode(ee.ExitCode())
		}
		return err
	}
	return nil
}
