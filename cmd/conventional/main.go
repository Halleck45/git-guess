// Command conventional guesses the Conventional Commits type of a change.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Halleck45/conventional/internal/gitx"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

const usage = `conventional — guess the Conventional Commits type of a change

Usage:
  conventional [flags]                 classify staged changes (or the working tree when nothing is staged)
  conventional [flags] <rev>           classify a commit or a range (HEAD~1, main..feature)
  git diff | conventional [flags]      classify a diff from stdin
  conventional commit [git commit args] commit with the type prefilled
  conventional hook install|uninstall  prefill the type on every git commit
  conventional check [<base>..<head>]  lint commit types semantically (for CI; --github for annotations)
  conventional eval [-n 200]           replay this repository's history and score the guesses
  conventional version

Flags:
  -m, --message <subject>   draft subject; prints a complete header "type(scope): subject"
  -n, --top <n>             show the n best candidates (default 3)
  -q, --quiet               print only the type
      --json                machine readable output
      --explain             show which features drove the decision
      --scope <name>        force the scope
      --no-scope            never add a scope
      --no-prior            do not adapt to this repository's history
      --staged              only staged changes (no fallback to the working tree)
      --unstaged            only unstaged changes
      --all                 everything since HEAD (staged and unstaged)
      --min-confidence <p>  exit 3 when the confidence is below p (0..1)
      --no-color            disable colors
  -h, --help                this help

Examples:
  conventional                     ✔ feat(auth)  ████████░░ 82%
  conventional -m "add login"      feat(auth): add login
  git commit -m "$(conventional -m "add login")"
  conventional commit -m "add login"
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		var ec exitCode
		if errors.As(err, &ec) {
			os.Exit(int(ec))
		}
		fmt.Fprintln(os.Stderr, "conventional:", err)
		if errors.Is(err, gitx.ErrNoChanges) {
			fmt.Fprintln(os.Stderr, "hint: stage some changes with git add, pass a revision, or pipe a diff on stdin")
		}
		os.Exit(1)
	}
}

type exitCode int

func (e exitCode) Error() string { return "exit " + strconv.Itoa(int(e)) }

func run(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	opts := classifyOptions{source: gitx.SourceStaged, top: 3, lambda: 0.7, fallback: true}
	var quiet, asJSON, noColor, explicitSource bool
	minConf := -1.0
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		// --flag=value and -nN forms
		if eq := strings.IndexByte(a, '='); eq > 0 && strings.HasPrefix(a, "--") {
			args = append(append(append([]string{}, args[:i]...), a[:eq], a[eq+1:]), args[i+1:]...)
			a = args[i]
		} else if len(a) > 2 && strings.HasPrefix(a, "-n") && !strings.HasPrefix(a, "--") {
			args = append(append(append([]string{}, args[:i]...), "-n", a[2:]), args[i+1:]...)
			a = args[i]
		}
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s needs a value", a)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case a == "-h" || a == "--help" || a == "help":
			fmt.Fprint(stdout, usage)
			return nil
		case a == "version" || a == "--version" || a == "-v":
			v := version
			if commit != "" {
				v += " (" + commit[:min(7, len(commit))] + ", " + date + ")"
			}
			fmt.Fprintln(stdout, "conventional", v)
			return nil
		case a == "-m" || a == "--message":
			opts.message, err = next()
		case strings.HasPrefix(a, "--message="):
			opts.message = strings.TrimPrefix(a, "--message=")
		case strings.HasPrefix(a, "-m") && len(a) > 2:
			opts.message = a[2:]
		case a == "-n" || a == "--top":
			var s string
			s, err = next()
			if err == nil {
				opts.top, err = strconv.Atoi(s)
			}
		case a == "-q" || a == "--quiet":
			quiet = true
		case a == "--json":
			asJSON = true
		case a == "--explain":
			opts.explain = true
		case a == "--scope":
			opts.scope, err = next()
		case strings.HasPrefix(a, "--scope="):
			opts.scope = strings.TrimPrefix(a, "--scope=")
		case a == "--no-scope":
			opts.noScope = true
		case a == "--no-prior":
			opts.noPrior = true
		case a == "--staged" || a == "--cached":
			opts.source, opts.fallback, explicitSource = gitx.SourceStaged, false, true
		case a == "--unstaged":
			opts.source, explicitSource = gitx.SourceUnstaged, true
		case a == "--all" || a == "-a":
			opts.source, explicitSource = gitx.SourceAll, true
		case a == "--min-confidence":
			var s string
			s, err = next()
			if err == nil {
				minConf, err = strconv.ParseFloat(s, 64)
			}
		case a == "--no-color":
			noColor = true
		case a == "commit":
			return runCommit(args[i+1:], opts, stdin, stdout, stderr)
		case a == "hook":
			return runHook(args[i+1:], stdout, stderr)
		case a == "eval":
			tty := isTerminal(os.Stdout)
			return runEval(args[i+1:], stdout, style{color: tty && !noColor && os.Getenv("NO_COLOR") == ""})
		case a == "check":
			tty := isTerminal(os.Stdout)
			return runCheck(args[i+1:], stdout, stderr, style{color: tty && !noColor && os.Getenv("NO_COLOR") == ""})
		case strings.HasPrefix(a, "-") && a != "-":
			return fmt.Errorf("unknown flag %s (see --help)", a)
		default:
			positional = append(positional, a)
		}
		if err != nil {
			return err
		}
	}
	if len(positional) > 1 {
		return fmt.Errorf("too many arguments: %v", positional)
	}
	if len(positional) == 1 {
		if positional[0] == "-" {
			opts.stdin, _ = io.ReadAll(stdin)
		} else {
			opts.rev = positional[0]
		}
	} else if !explicitSource && !isTerminal(stdin) {
		b, _ := io.ReadAll(stdin)
		if len(strings.TrimSpace(string(b))) > 0 {
			opts.stdin = b
		}
	}
	res, err := classify(opts)
	if err != nil {
		return err
	}
	tty := isTerminal(os.Stdout)
	st := style{color: tty && !noColor && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"}
	render(stdout, res, quiet, asJSON, tty, st)
	if minConf >= 0 && res.Confidence < minConf {
		return exitCode(3)
	}
	return nil
}
