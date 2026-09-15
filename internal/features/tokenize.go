package features

import (
	"strings"
	"unicode"
)

// Tokenizer contract (shared by training and inference):
//   - identifiers are split on non-alphanumerics and on camelCase boundaries
//   - tokens are lower-cased
//   - runs of digits become the token "0"
//   - tokens shorter than 2 or longer than 24 characters are dropped (numbers kept)
//   - at most MaxTokensPerLine tokens are produced per line
const MaxTokensPerLine = 40

// tokenizeInto appends tokens of s to dst and returns it.
func tokenizeInto(dst []string, s string) []string {
	n := 0
	start := -1
	prevLower := false
	prevUpper := false
	prevDigit := false
	flush := func(end int) {
		if start < 0 {
			return
		}
		tok := s[start:end]
		start = -1
		if n >= MaxTokensPerLine {
			return
		}
		if isDigits(tok) {
			dst = append(dst, "0")
			n++
			return
		}
		if len(tok) < 2 || len(tok) > 24 {
			return
		}
		dst = append(dst, strings.ToLower(tok))
		n++
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isLetter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
		isDigit := c >= '0' && c <= '9'
		switch {
		case isLetter || isDigit:
			isUpper := c >= 'A' && c <= 'Z'
			if start < 0 {
				start = i
			} else if isUpper && prevLower { // camelCase boundary
				flush(i)
				start = i
			} else if isUpper && prevUpper && i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z' { // HTTPServer -> HTTP Server
				flush(i)
				start = i
			} else if isDigit != prevDigit { // letter/digit boundary
				flush(i)
				start = i
			}
			prevLower = c >= 'a' && c <= 'z'
			prevUpper = isUpper
			prevDigit = isDigit
		default:
			flush(i)
			prevLower, prevUpper, prevDigit = false, false, false
		}
		if n >= MaxTokensPerLine {
			start = -1
			break
		}
	}
	flush(len(s))
	return dst
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// lineShape returns a compact descriptor of the syntactic shape of a code
// line: its first "word" (keyword, identifier or punctuation class).
func lineShape(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "<empty>"
	}
	if isCommentLine(s) {
		return "<comment>"
	}
	// first word
	end := 0
	for end < len(s) && (s[end] >= 'a' && s[end] <= 'z' || s[end] >= 'A' && s[end] <= 'Z' || s[end] == '_' || s[end] == '@' || s[end] == '#' || s[end] == '$') {
		end++
	}
	if end > 0 {
		w := strings.ToLower(s[:end])
		if len(w) > 16 {
			w = w[:16]
		}
		return w
	}
	// punctuation class
	switch s[0] {
	case '}', ')', ']':
		return "<close>"
	case '{', '(', '[':
		return "<open>"
	case '<':
		return "<tag>"
	case '"', '\'', '`':
		return "<string>"
	case '.':
		return "<dot>"
	case '*', '-', '+', '|', '=', '>', '/', '\\', ':', ';', ',', '?', '!', '&', '%', '^', '~':
		return "<punct>"
	default:
		if s[0] >= '0' && s[0] <= '9' {
			return "<num>"
		}
		return "<other>"
	}
}

func isCommentLine(s string) bool {
	return strings.HasPrefix(s, "//") || strings.HasPrefix(s, "#") && !strings.HasPrefix(s, "#include") && !strings.HasPrefix(s, "#!") && !strings.HasPrefix(s, "#[") && !strings.HasPrefix(s, "#define") && !strings.HasPrefix(s, "#if") && !strings.HasPrefix(s, "#endif") && !strings.HasPrefix(s, "#pragma") && !strings.HasPrefix(s, "#else") && !strings.HasPrefix(s, "#{") ||
		strings.HasPrefix(s, "/*") || strings.HasPrefix(s, "* ") || s == "*" || strings.HasPrefix(s, "*/") || strings.HasPrefix(s, "'''") || strings.HasPrefix(s, "\"\"\"") ||
		strings.HasPrefix(s, "--") && !strings.HasPrefix(s, "---") || strings.HasPrefix(s, "///") || strings.HasPrefix(s, "<!--") || strings.HasPrefix(s, "%") && len(s) > 1 && s[1] == ' ' ||
		strings.HasPrefix(s, ";;") || strings.HasPrefix(s, "\"\"") && len(s) > 2 && s[2] != '"' || strings.HasPrefix(s, "REM ") || strings.HasPrefix(s, "rem ")
}

// normalizeWS collapses whitespace so that formatting-only changes compare
// equal.
func normalizeWS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isImportLine detects dependency/import statements across languages.
func isImportLine(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "import ") || strings.HasPrefix(s, "from ") && strings.Contains(s, " import ") ||
		strings.HasPrefix(s, "require(") || strings.Contains(s, "require(\"") || strings.Contains(s, "require('") ||
		strings.HasPrefix(s, "use ") || strings.HasPrefix(s, "#include") || strings.HasPrefix(s, "using ") ||
		strings.HasPrefix(s, "require \"") || strings.HasPrefix(s, "require '") || strings.HasPrefix(s, "require_relative") ||
		strings.HasPrefix(s, "import(") || strings.HasPrefix(s, "@import") || strings.HasPrefix(s, "export * from") ||
		strings.HasPrefix(s, "export {") && strings.Contains(s, " from ") || strings.HasPrefix(s, "extern crate") || strings.HasPrefix(s, "package ")
}
