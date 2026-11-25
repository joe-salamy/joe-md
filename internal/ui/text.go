package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// fit truncates or pads a styled line to exactly w cells.
func fit(s string, w int) string {
	width := cellWidth(s)
	if width > w {
		if cut, ok := cutPrefix(s, w); ok {
			s, width = cut, w
		} else {
			s = ansi.Truncate(s, w, "")
			width = ansi.StringWidth(s)
		}
	}
	pad := w - width
	return s + "\x1b[0m" + strings.Repeat(" ", max(pad, 0))
}

// cellWidth is ansi.StringWidth, with a fast path for the printable lines
// that most of a frame is made of. Every line drawn is measured several
// times, and the ansi package's grapheme segmentation costs about 15ns a
// byte, which is real work on a small laptop; counting cells is not.
func cellWidth(s string) int {
	if w, ok := plainWidth(s); ok {
		return w
	}
	return ansi.StringWidth(s)
}

// escByte starts every ANSI escape sequence.
const escByte = 0x1b

// plainWidth is s's width in cells when s is plain text with ANSI escape
// sequences: printable one-cell runes, each one cell wide. ok is false for
// anything else — control characters, wide runes, combining marks — which
// only the ansi package can measure.
func plainWidth(s string) (int, bool) {
	width := 0
	ok := eachCell(s, func(int, int) bool {
		width++
		return false
	})
	return width, ok
}

// eachCell walks s's printable cells in order, calling fn with each one's
// byte offset and length, and stops early when fn returns true. It reports
// false when s holds something the fast paths cannot count.
func eachCell(s string, fn func(off, size int) bool) bool {
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == escByte:
			n := escLen(s[i:])
			if n == 0 {
				return false
			}
			i += n
		case c >= ' ' && c < 0x7f:
			if fn(i, 1) {
				return true
			}
			i++
		case c < utf8.RuneSelf:
			return false // a control character or DEL
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if !oneCell(r) {
				return false
			}
			if fn(i, size) {
				return true
			}
			i += size
		}
	}
	return true
}

// oneCell reports whether r is a cell of its own: one column wide, and
// neither wide nor combining nor joined to its neighbour by a grapheme rule.
// These are the runes that turn up in documents and frames — accented
// letters, typographic quotes and dashes, box drawing — and anything else
// (CJK, emoji, marks) is left to the ansi package.
func oneCell(r rune) bool {
	switch {
	case r >= 0x00a0 && r < 0x00ad, // Latin-1 spaces, letters and symbols
		r > 0x00ad && r <= 0x024f,  // the rest of Latin-1, Latin extended
		r >= 0x2010 && r <= 0x2027, // dashes, quotes, bullets, the ellipsis
		r >= 0x2030 && r <= 0x205e, // angle quotes, primes, fractions
		r >= 0x2500 && r <= 0x257f: // box drawing
		return true
	}
	return false
}

// cutPrefix truncates s to w cells when plainWidth measures it. The escape
// sequences after the cut go too: fit's reset stands in for them.
func cutPrefix(s string, w int) (string, bool) {
	width, cut := 0, len(s)
	ok := eachCell(s, func(off, size int) bool {
		if width == w {
			cut = off
			return true
		}
		width++
		return false
	})
	if !ok {
		return "", false
	}
	return s[:cut], true
}

// cutCells splits s at the cells a and b when plainWidth measures it: left is
// s up to column a, right starts at column b. ok is false when the line needs
// the ansi package's Unicode-aware cutting.
func cutCells(s string, a, b int) (left, right string, ok bool) {
	if a < 0 || b < a {
		return "", "", false
	}
	width := 0
	i, gotI := len(s), a == 0
	if gotI {
		i = 0
	}
	j := len(s)
	ok = eachCell(s, func(off, size int) bool {
		if !gotI && width == a {
			i, gotI = off, true
		}
		if width == b {
			j = off
			return true
		}
		width++
		return false
	})
	if !ok {
		return "", "", false
	}
	return s[:i], s[j:], true
}

// escLen is the byte length of the ANSI escape sequence at the start of s,
// which must start with ESC, or 0 when it is not one this file can step over
// (the ansi package parses anything else).
func escLen(s string) int {
	if len(s) < 2 || s[0] != escByte {
		return 0
	}
	switch s[1] {
	case '[': // CSI: parameters, then a final byte
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
			if s[i] < 0x20 || s[i] > 0x7e {
				return 0
			}
		}
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: BEL or ST ends them
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == escByte && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	default: // a bare two-byte escape, like ESC 7
		if s[1] >= 0x40 && s[1] <= 0x5f {
			return 2
		}
	}
	return 0
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
