package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// fit truncates or pads a styled line to exactly w cells.
func fit(s string, w int) string {
	width := ansi.StringWidth(s)
	if width > w {
		s = ansi.Truncate(s, w, "")
		width = ansi.StringWidth(s)
	}
	pad := w - width
	return s + "\x1b[0m" + strings.Repeat(" ", max(pad, 0))
}

// plural is "1 thing" or "n things".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// tildePath shortens a path in the home directory to start with ~.
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if strings.HasPrefix(p, home+string(filepath.Separator)) {
			return "~" + p[len(home):]
		}
	}
	return p
}

// sanitizeLine replaces control characters (notably ESC, which starts ANSI
// and OSC sequences) with spaces, so text from a filename, a document or an
// error message cannot drive the terminal or break a single-line row.
func sanitizeLine(s string) string {
	clean := true
	for _, r := range s {
		if unicode.IsControl(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// stripControlBytes replaces control bytes with spaces, keeping the byte
// length so byte offsets (search spans) stay valid. It is for text already
// known to be one line.
func stripControlBytes(s string) string {
	clean := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f || s[i] == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] <= 0x9f {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] < 0x20 || b[i] == 0x7f:
			b[i] = ' '
		case b[i] == 0xc2 && i+1 < len(b) && b[i+1] >= 0x80 && b[i+1] <= 0x9f:
			b[i], b[i+1] = ' ', ' ' // C1 control in UTF-8
			i++
		}
	}
	return string(b)
}
