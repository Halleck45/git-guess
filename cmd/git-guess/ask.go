package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// askThreshold is the confidence under which the user is offered a choice
// between the two best candidates, when a terminal is available.
const askThreshold = 0.55

// maybeAsk lets the user pick between the two best types when the guess is
// uncertain. It reads from /dev/tty so it also works inside git hooks. Any
// failure (no terminal, CI, empty answer) keeps the guess.
func maybeAsk(res *Result, stderr *os.File) {
	if res.Confidence >= askThreshold || len(res.Candidates) < 2 || os.Getenv("GIT_GUESS_ASK") == "0" || os.Getenv("CI") != "" {
		return
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer tty.Close()
	a, b := res.Candidates[0], res.Candidates[1]
	st := style{color: true}
	fmt.Fprintf(tty, "%s %s %s or %s %s? [%s/%s, Enter keeps %s] ", st.yellow("?"),
		st.bold(a.Type), st.dim(percent(a.P)), st.bold(b.Type), st.dim(percent(b.P)), string(a.Type[0]), string(b.Type[0]), a.Type)
	line, _ := bufio.NewReader(tty).ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	switch {
	case ans == "" || strings.HasPrefix(a.Type, ans):
		return
	case strings.HasPrefix(b.Type, ans):
		res.Type = b.Type
		res.Confidence = b.P
	default:
		for _, c := range res.Candidates {
			if c.Type == ans {
				res.Type, res.Confidence = c.Type, c.P
			}
		}
	}
	res.Header = res.formatHeader(strings.TrimSpace(strings.TrimPrefix(res.Header, strings.SplitN(res.Header, ":", 2)[0]+":")))
}
