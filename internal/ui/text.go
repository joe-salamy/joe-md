package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// fit truncates or pads a styled line to exactly w cells.
func fit(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "")
	}
	pad := w - ansi.StringWidth(s)
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
