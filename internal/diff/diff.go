// Package diff parses unified diffs as produced by git (git diff, git show,
// git log -p) into a lightweight structure used by the featurizer.
//
// The parser is deliberately tolerant: it never fails on malformed input, it
// simply skips what it does not understand. The same parser is used at
// training time (on stored patches) and at inference time (on `git diff`
// output), which guarantees feature parity.
package diff

import (
	"bufio"
	"io"
	"strings"
)

// Status of a file inside a diff.
type Status uint8

const (
	Modified Status = iota
	Added
	Deleted
	Renamed
	Copied
)

func (s Status) String() string {
	switch s {
	case Added:
		return "added"
	case Deleted:
		return "deleted"
	case Renamed:
		return "renamed"
	case Copied:
		return "copied"
	default:
		return "modified"
	}
}

// Line is one changed line of a hunk (context lines are dropped).
type Line struct {
	Added bool // true for '+', false for '-'
	Text  string
}

// File is one file entry of a diff.
type File struct {
	Path    string // new path (or old path when deleted)
	OldPath string // set for renames/copies
	Status  Status
	Binary  bool
	Lines   []Line   // changed lines, in order, capped by MaxLinesPerFile
	Hunks   []string // hunk header contexts ("func foo()" part of @@ lines)
	Added   int      // total added lines (not capped)
	Removed int      // total removed lines (not capped)
	Mode    string   // file mode when known (e.g. "100755")
}

// Diff is a parsed unified diff.
type Diff struct {
	Files     []File
	Truncated bool // some content was dropped because of the caps
}

// Caps applied at parse time. They are part of the model contract: changing
// them changes the features, so the model must be retrained.
const (
	MaxFiles        = 200
	MaxLinesPerFile = 400
	MaxLineLen      = 1000
)

// Parse reads a unified diff. It never returns an error for malformed content.
func Parse(r io.Reader) (*Diff, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	d := &Diff{}
	var cur *File
	inBody := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --cc ") || strings.HasPrefix(line, "diff --combined "):
			// Combined diff of a merge: we only learn the path; hunks use a
			// different syntax and are ignored.
			if len(d.Files) >= MaxFiles {
				d.Truncated = true
				cur = nil
				inBody = false
				continue
			}
			d.Files = append(d.Files, File{})
			cur = &d.Files[len(d.Files)-1]
			cur.Path = cleanPath(strings.TrimPrefix(strings.TrimPrefix(line, "diff --cc "), "diff --combined "))
			inBody = false
		case strings.HasPrefix(line, "diff --git "):
			if len(d.Files) >= MaxFiles {
				d.Truncated = true
				cur = nil
				inBody = false
				continue
			}
			d.Files = append(d.Files, File{})
			cur = &d.Files[len(d.Files)-1]
			cur.Path, cur.OldPath = splitGitHeader(line[len("diff --git "):])
			cur.OldPath = ""
			inBody = false
		case cur == nil:
			// Diffs without "diff --git" headers (plain `diff -u`): synthesize.
			if strings.HasPrefix(line, "--- ") {
				if len(d.Files) >= MaxFiles {
					d.Truncated = true
					continue
				}
				d.Files = append(d.Files, File{})
				cur = &d.Files[len(d.Files)-1]
				cur.Path = cleanPath(line[4:])
				if cur.Path == "" { // --- /dev/null
					cur.Status = Added
				}
				inBody = false
			}
		case !inBody && strings.HasPrefix(line, "new file mode "):
			cur.Status = Added
			cur.Mode = strings.TrimSpace(line[len("new file mode "):])
		case !inBody && strings.HasPrefix(line, "deleted file mode "):
			cur.Status = Deleted
		case !inBody && strings.HasPrefix(line, "new mode "):
			cur.Mode = strings.TrimSpace(line[len("new mode "):])
		case !inBody && strings.HasPrefix(line, "rename from "):
			cur.OldPath = unquote(line[len("rename from "):])
			cur.Status = Renamed
		case !inBody && strings.HasPrefix(line, "rename to "):
			cur.Path = unquote(line[len("rename to "):])
		case !inBody && strings.HasPrefix(line, "copy from "):
			cur.OldPath = unquote(line[len("copy from "):])
			cur.Status = Copied
		case !inBody && strings.HasPrefix(line, "copy to "):
			cur.Path = unquote(line[len("copy to "):])
		case !inBody && strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case !inBody && strings.HasPrefix(line, "GIT binary patch"):
			cur.Binary = true
			inBody = true
		case !inBody && strings.HasPrefix(line, "--- "):
			p := cleanPath(line[4:])
			if p == "" { // /dev/null
				cur.Status = Added
			}
		case !inBody && strings.HasPrefix(line, "+++ "):
			p := cleanPath(line[4:])
			if p == "" {
				cur.Status = Deleted
			} else if cur.Path == "" {
				cur.Path = p
			}
		case !inBody && (strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "similarity index") ||
			strings.HasPrefix(line, "dissimilarity index") || strings.HasPrefix(line, "old mode ")):
			// ignore
		case strings.HasPrefix(line, "@@"):
			inBody = true
			if i := strings.Index(line[2:], "@@"); i >= 0 {
				ctx := strings.TrimSpace(line[2+i+2:])
				if ctx != "" && len(cur.Hunks) < 64 {
					cur.Hunks = append(cur.Hunks, ctx)
				}
			}
		case inBody && len(line) > 0 && (line[0] == '+' || line[0] == '-'):
			if cur.Binary {
				continue
			}
			if line[0] == '+' {
				cur.Added++
			} else {
				cur.Removed++
			}
			if len(cur.Lines) >= MaxLinesPerFile {
				d.Truncated = true
				continue
			}
			text := line[1:]
			if len(text) > MaxLineLen {
				text = text[:MaxLineLen]
				d.Truncated = true
			}
			cur.Lines = append(cur.Lines, Line{Added: line[0] == '+', Text: text})
		case inBody && strings.HasPrefix(line, "\\ No newline"):
			// ignore
		}
	}
	return d, sc.Err()
}

// ParseString is a convenience wrapper around Parse.
func ParseString(s string) *Diff {
	d, _ := Parse(strings.NewReader(s))
	return d
}

// splitGitHeader parses `a/old b/new` (possibly quoted) from a diff --git line.
func splitGitHeader(s string) (newPath, oldPath string) {
	s = strings.TrimSpace(s)
	// Quoted paths: "a/x y" "b/x y"
	if strings.HasPrefix(s, "\"") {
		parts := splitQuoted(s)
		if len(parts) == 2 {
			return stripPrefix(parts[1]), stripPrefix(parts[0])
		}
	}
	// Common case: "a/path b/path". Paths may contain spaces; find the split
	// where the left starts with a/ and the right with b/.
	hasPrefix := len(s) >= 2 && s[1] == '/'
	for i := 1; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		l, r := s[:i], s[i+1:]
		if hasPrefix && len(r) >= 2 && r[1] == '/' && stripPrefix(l) == stripPrefix(r) {
			return stripPrefix(r), stripPrefix(l)
		}
		if !hasPrefix && l == r { // diff.noprefix=true
			return r, l
		}
	}
	// Fallback: last occurrence of " b/".
	if i := strings.LastIndex(s, " b/"); i >= 0 {
		return stripPrefix(s[i+1:]), stripPrefix(s[:i])
	}
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		return stripPrefix(fields[len(fields)-1]), stripPrefix(fields[0])
	}
	return stripPrefix(s), ""
}

func splitQuoted(s string) []string {
	var out []string
	var cur strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			if in {
				out = append(out, cur.String())
				cur.Reset()
			}
			in = !in
		case c == '\\' && in && i+1 < len(s):
			i++
			if i+2 < len(s) && isOctal(s[i]) && isOctal(s[i+1]) && isOctal(s[i+2]) {
				cur.WriteByte((s[i]-'0')<<6 | (s[i+1]-'0')<<3 | (s[i+2] - '0'))
				i += 2
				continue
			}
			switch s[i] {
			case 'n':
				cur.WriteByte('\n')
			case 't':
				cur.WriteByte('\t')
			default:
				cur.WriteByte(s[i])
			}
		case in:
			cur.WriteByte(c)
		}
	}
	return out
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

// stripPrefix removes the one-letter prefix git adds (a/, b/, and the
// mnemonic i/, w/, c/, o/ variants).
func stripPrefix(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && p[1] == '/' && (p[0] == 'a' || p[0] == 'b' || p[0] == 'i' || p[0] == 'w' || p[0] == 'c' || p[0] == 'o') {
		return p[2:]
	}
	return p
}

// unquote handles quoted paths on rename/copy lines.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "\"") {
		if q := splitQuoted(s); len(q) == 1 {
			return q[0]
		}
	}
	return s
}

// cleanPath handles "--- a/path\tdate" and /dev/null.
func cleanPath(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(s, "\"") {
		if q := splitQuoted(s); len(q) == 1 {
			s = q[0]
		}
	}
	return stripPrefix(s)
}
