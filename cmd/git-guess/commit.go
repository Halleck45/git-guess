package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Halleck45/git-guess/internal/gitx"
	"github.com/Halleck45/git-guess/internal/model"
)

// runCommit wraps `git commit`. A -m subject gets the header prefilled;
// without -m the editor opens on a template starting with the header.
func runCommit(args []string, opts classifyOptions, stdin *os.File, stdout, stderr io.Writer) error {
	subject, rest, hasAll, err := parseCommitArgs(args)
	if err != nil {
		return err
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
	maybeAsk(res, os.Stderr)
	if subject != "" {
		header := res.formatHeader(subject)
		fmt.Fprintln(stderr, header)
		return execGit(append([]string{"commit", "-m", header}, rest...), stdin, stdout, stderr)
	}
	tmp, err := os.CreateTemp("", "git-guess-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	fmt.Fprintf(tmp, "%s \n", res.formatHeader(""))
	if ch, strip := gitx.CommentChar(); strip {
		fmt.Fprintf(tmp, "%s git guess: %s (%s)", ch, res.Type, percent(res.Confidence))
		if len(res.Candidates) > 1 {
			fmt.Fprintf(tmp, ", also %s (%s)", res.Candidates[1].Type, percent(res.Candidates[1].P))
		}
		fmt.Fprintln(tmp)
	}
	tmp.Close()
	return execGit(append([]string{"commit", "--template", filepath.ToSlash(tmp.Name())}, rest...), stdin, stdout, stderr)
}

// parseCommitArgs splits git commit arguments into the first -m subject,
// the arguments passed through, and whether -a/--all was given.
func parseCommitArgs(args []string) (subject string, rest []string, hasAll bool, err error) {
	setSubject := func(v string) {
		if subject == "" {
			subject = v
		} else { // further -m are body paragraphs, passed through
			rest = append(rest, "-m", v)
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			rest = append(rest, args[i:]...)
			i = len(args)
		case a == "--message":
			if i+1 >= len(args) {
				return "", nil, false, fmt.Errorf("--message needs a value")
			}
			i++
			setSubject(args[i])
		case strings.HasPrefix(a, "--message="):
			setSubject(strings.TrimPrefix(a, "--message="))
		case a == "--all":
			hasAll = true
			rest = append(rest, a)
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 1:
			// short option cluster such as -am "msg", -sam "msg", -qa
			cl := a[1:]
			if j := strings.IndexByte(cl, 'm'); j >= 0 {
				if j+1 < len(cl) {
					setSubject(cl[j+1:])
				} else if i+1 < len(args) {
					i++
					setSubject(args[i])
				} else {
					return "", nil, false, fmt.Errorf("-m needs a value")
				}
				cl = cl[:j]
			}
			if strings.ContainsRune(cl, 'a') {
				hasAll = true
			}
			if cl != "" {
				rest = append(rest, "-"+cl)
			}
		default:
			rest = append(rest, a)
		}
	}
	return subject, rest, hasAll, nil
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
