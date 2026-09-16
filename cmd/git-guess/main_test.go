package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null", "HOME="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func tempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n"), 0o644)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "chore: init")
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })
	return dir
}

func TestRunStaged(t *testing.T) {
	dir := tempRepo(t)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("# Guide\n\nHow to use the thing.\n"), 0o644)
	git(t, dir, "add", ".")
	var out bytes.Buffer
	if err := run([]string{"--json", "--no-prior"}, os.Stdin, &out, &out); err != nil {
		t.Fatal(err)
	}
	var res Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out.String())
	}
	if res.Type != "docs" || res.Source != "staged" || res.Files != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	// quiet output is just the type
	out.Reset()
	if err := run([]string{"-q"}, os.Stdin, &out, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "docs" {
		t.Errorf("quiet: %q", out.String())
	}
	// with a message, non-tty output is a full header
	out.Reset()
	if err := run([]string{"-m", "explain the thing"}, os.Stdin, &out, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "docs: explain the thing" {
		t.Errorf("header: %q", out.String())
	}
}

func TestRunNoChanges(t *testing.T) {
	tempRepo(t)
	var out bytes.Buffer
	err := run([]string{}, os.Stdin, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "no changes") {
		t.Fatalf("expected no changes error, got %v", err)
	}
}

func TestRunRevision(t *testing.T) {
	var out bytes.Buffer
	tempRepo(t)
	if err := run([]string{"HEAD", "--json"}, os.Stdin, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"source": "HEAD"`) {
		t.Errorf("%s", out.String())
	}
}

func TestHookRun(t *testing.T) {
	dir := tempRepo(t)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n\nMore words here.\n"), 0o644)
	git(t, dir, "add", ".")
	msg := filepath.Join(dir, "MSG")
	// -m without a type: prefixed
	os.WriteFile(msg, []byte("update readme\n"), 0o644)
	if err := hookRun([]string{msg, "message"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(msg)
	if !strings.HasPrefix(string(b), "docs: update readme") {
		t.Errorf("hook message: %q", b)
	}
	// already conventional: untouched
	os.WriteFile(msg, []byte("fix: something\n"), 0o644)
	hookRun([]string{msg, "message"}, &bytes.Buffer{})
	b, _ = os.ReadFile(msg)
	if string(b) != "fix: something\n" {
		t.Errorf("hook touched a conventional message: %q", b)
	}
	// interactive commit: template prefilled
	os.WriteFile(msg, []byte("\n# Please enter the commit message\n"), 0o644)
	hookRun([]string{msg, ""}, &bytes.Buffer{})
	b, _ = os.ReadFile(msg)
	if !strings.HasPrefix(string(b), "docs: \n") || !strings.Contains(string(b), "# git guess: docs") {
		t.Errorf("hook template: %q", b)
	}
	// merge: untouched
	os.WriteFile(msg, []byte("Merge branch 'x'\n"), 0o644)
	hookRun([]string{msg, "merge"}, &bytes.Buffer{})
	b, _ = os.ReadFile(msg)
	if string(b) != "Merge branch 'x'\n" {
		t.Errorf("hook touched merge: %q", b)
	}
}

func TestHookInstall(t *testing.T) {
	dir := tempRepo(t)
	var out bytes.Buffer
	if err := runHook([]string{"install"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ".git", "hooks", "prepare-commit-msg")
	if b, err := os.ReadFile(p); err != nil || !strings.Contains(string(b), hookMarker) {
		t.Fatalf("hook not written: %v", err)
	}
	if err := runHook([]string{"uninstall"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("hook not removed")
	}
	os.WriteFile(p, []byte("#!/bin/sh\necho custom\n"), 0o755)
	if err := runHook([]string{"install"}, &out, &out); err == nil {
		t.Fatal("expected refusal on foreign hook")
	}
}

func TestParseCommitArgs(t *testing.T) {
	cases := []struct {
		in      []string
		subject string
		rest    []string
		all     bool
	}{
		{[]string{"-m", "add login"}, "add login", nil, false},
		{[]string{"-am", "add login"}, "add login", []string{"-a"}, true},
		{[]string{"-sam", "add login", "--no-verify"}, "add login", []string{"-sa", "--no-verify"}, true},
		{[]string{"-qa"}, "", []string{"-qa"}, true},
		{[]string{"--all", "-m", "x"}, "x", []string{"--all"}, true},
		{[]string{"-m", "subject", "-m", "body"}, "subject", []string{"-m", "body"}, false},
		{[]string{"-mvalue"}, "value", nil, false},
		{[]string{"--message=x", "--", "file"}, "x", []string{"--", "file"}, false},
	}
	for _, c := range cases {
		s, rest, all, err := parseCommitArgs(c.in)
		if err != nil || s != c.subject || all != c.all || strings.Join(rest, " ") != strings.Join(c.rest, " ") {
			t.Errorf("%v: got (%q, %v, %v, %v) want (%q, %v, %v)", c.in, s, rest, all, err, c.subject, c.rest, c.all)
		}
	}
}

func TestHookEdgeCases(t *testing.T) {
	dir := tempRepo(t)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n\nMore words here.\n"), 0o644)
	git(t, dir, "add", ".")
	msg := filepath.Join(dir, "MSG")
	check := func(content, source, sha, wantPrefix string) {
		t.Helper()
		os.WriteFile(msg, []byte(content), 0o644)
		args := []string{msg, source}
		if sha != "" {
			args = append(args, sha)
		}
		hookRun(args, &bytes.Buffer{})
		b, _ := os.ReadFile(msg)
		if !strings.HasPrefix(string(b), wantPrefix) {
			t.Errorf("source=%q content=%q: got %q, want prefix %q", source, content, b, wantPrefix)
		}
	}
	check("#123 update readme\n", "message", "", "docs: #123 update readme")
	check("fixup! docs: x\n", "message", "", "fixup! docs: x")
	check("squash! something\n", "message", "", "squash! something")
	check("Revert \"docs: x\"\n", "message", "", "Revert \"docs: x\"")
	check("amended\n", "message", "abc123", "amended")
	check("docs: \n# comment\n", "template", "", "docs: \n")
	check("docs:\n", "template", "", "docs:\n")
	check("subject\n\nbody paragraph\n", "message", "", "docs: subject\n\nbody paragraph")
}

func TestRunStagedIgnoresStdinWhenExplicit(t *testing.T) {
	dir := tempRepo(t)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n\nMore.\n"), 0o644)
	git(t, dir, "add", ".")
	r, w, _ := os.Pipe()
	w.WriteString("diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n")
	// w stays open: reading stdin would block
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- run([]string{"--staged", "--json"}, r, &out, &out) }()
	select {
	case err := <-done:
		if err != nil || !strings.Contains(out.String(), `"source": "staged"`) {
			t.Fatalf("err=%v out=%s", err, out.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run blocked on stdin despite --staged")
	}
	w.Close()
}

func TestFlagForms(t *testing.T) {
	dir := tempRepo(t)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n\nMore.\n"), 0o644)
	git(t, dir, "add", ".")
	var out bytes.Buffer
	if err := run([]string{"--top=1", "--json", "--min-confidence=0"}, os.Stdin, &out, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), `"type"`) != 2 { // one at top level, one candidate
		t.Errorf("--top=1 not honored: %s", out.String())
	}
	out.Reset()
	if err := run([]string{"-n2", "--json"}, os.Stdin, &out, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), `"type"`) != 3 {
		t.Errorf("-n2 not honored: %s", out.String())
	}
}

func TestCheck(t *testing.T) {
	dir := tempRepo(t)
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# a\n\nwords\n"), 0o644)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "docs: add a")
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b\n\nwords\n"), 0o644)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "add b without a type")
	var out bytes.Buffer
	err := runCheck([]string{"HEAD~2..HEAD", "--json"}, &out, &out, style{})
	if err == nil {
		t.Fatalf("expected exit 1 for a commit without type\n%s", out.String())
	}
	var res struct {
		Missing int `json:"missing"`
		Commits []checkResult
	}
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil || res.Missing != 1 || len(res.Commits) != 2 {
		t.Fatalf("bad check output (%v): %s", jerr, out.String())
	}
	if res.Commits[0].Status != "ok" || res.Commits[1].Status != "missing" || res.Commits[1].Guess == "" {
		t.Errorf("statuses: %+v", res.Commits)
	}
}
