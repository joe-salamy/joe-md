package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// lines covers what a frame is made of: styled document text, plain text,
// Unicode the fast path leaves to the ansi package, and escapes it has to
// step over.
var lines = []string{
	"",
	" ",
	"plain ascii",
	"\x1b[38;5;252mstyled\x1b[0m",
	"\x1b[1;31mbold red\x1b[0m and \x1b[4munderlined\x1b[0m",
	"\x1b[38;2;10;20;30mtruecolor\x1b[m",
	"\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\",
	"\x1b]0;title\x07after",
	"\x1b[1m\x1b[0m",
	"tab\there",
	"line\nbreak",
	"\x7fdel",
	"café",
	"日本語の見出し",
	"👨‍👩‍👧 family",
	"e\u0301 combining",
	"\x1b[31m日本語\x1b[0m",
}

// Measuring must agree with the ansi package, which handles the Unicode the
// fast path passes to it.
func TestCellWidthMatchesAnsi(t *testing.T) {
	for _, s := range lines {
		if got, want := cellWidth(s), ansi.StringWidth(s); got != want {
			t.Errorf("cellWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

// plainWidth is exact for plain one-cell text, and refuses anything else so
// the ansi package measures it.
func TestPlainWidth(t *testing.T) {
	for _, s := range []string{"", "abc", "a b", "│", "café", "— “quoted” …", "\x1b[1mx\x1b[0m"} {
		want := ansi.StringWidth(s)
		if got, ok := plainWidth(s); !ok || got != want {
			t.Errorf("plainWidth(%q) = %d, %v, want %d, true", s, got, ok, want)
		}
	}
	for _, s := range []string{"日本", "👨‍👩‍👧", "e\u0301", "a\tb", "a\x7fb", "\x1b[999"} {
		if got, ok := plainWidth(s); ok {
			t.Errorf("plainWidth(%q) = %d, true, want the ansi fallback", s, got)
		}
	}
}

// Every rune oneCell accepts must be one cell wide to the ansi package, and
// must not cluster with its neighbours.
func TestOneCell(t *testing.T) {
	for r := rune(0xa0); r <= 0x2580; r++ {
		ok := oneCell(r)
		alone := ansi.StringWidth(string(r))
		if ok && alone != 1 {
			t.Errorf("oneCell(%#U) but ansi measures %d", r, alone)
		}
		if ok && ansi.StringWidth("a"+string(r)+"b") != 3 {
			t.Errorf("%#U clusters with its neighbours", r)
		}
		if !ok && alone == 1 && r >= 0x2500 && r <= 0x257f {
			t.Errorf("oneCell(%#U) should be true: one cell", r)
		}
	}
}

func TestCutPrefixMatchesTruncate(t *testing.T) {
	for _, s := range lines {
		width := ansi.StringWidth(s)
		for w := 0; w <= width+2; w++ {
			cut, ok := cutPrefix(s, w)
			if !ok {
				continue // left to ansi.Truncate
			}
			if got := ansi.StringWidth(cut); got > w {
				t.Errorf("cutPrefix(%q, %d) = %q, %d cells", s, w, ansi.Strip(cut), got)
			}
			want := ansi.Truncate(s, w, "")
			if ansi.Strip(cut) != ansi.Strip(want) {
				t.Errorf("cutPrefix(%q, %d) = %q, want %q", s, w, ansi.Strip(cut), ansi.Strip(want))
			}
		}
	}
}

func TestCutCellsMatchesAnsi(t *testing.T) {
	for _, s := range lines {
		width := ansi.StringWidth(s)
		for a := 0; a <= width+1; a++ {
			for b := a; b <= width+1; b++ {
				left, right, ok := cutCells(s, a, b)
				if !ok {
					continue // left to ansi.Truncate and TruncateLeft
				}
				if got := ansi.StringWidth(left); got != min(a, width) {
					t.Errorf("cutCells(%q, %d, %d) left is %d cells", s, a, b, got)
				}
				if got := ansi.StringWidth(right); got != width-min(b, width) {
					t.Errorf("cutCells(%q, %d, %d) right is %d cells", s, a, b, got)
				}
				wantLeft := ansi.Truncate(s, a, "")
				wantRight := ansi.TruncateLeft(s, b, "")
				if ansi.Strip(left) != ansi.Strip(wantLeft) {
					t.Errorf("cutCells(%q, %d, %d) left = %q, want %q", s, a, b, ansi.Strip(left), ansi.Strip(wantLeft))
				}
				if ansi.Strip(right) != ansi.Strip(wantRight) {
					t.Errorf("cutCells(%q, %d, %d) right = %q, want %q", s, a, b, ansi.Strip(right), ansi.Strip(wantRight))
				}
			}
		}
	}
}

// fit pads and cuts to exactly w cells, styled or not.
func TestFit(t *testing.T) {
	for _, s := range lines {
		for _, w := range []int{0, 1, 5, 20} {
			got := fit(s, w)
			if n := ansi.StringWidth(got); n != w {
				t.Errorf("fit(%q, %d) is %d cells wide", s, w, n)
			}
			if ansi.StringWidth(s) <= w && !strings.HasPrefix(got, s) {
				t.Errorf("fit(%q, %d) = %q, want the line itself, padded", s, w, got)
			}
		}
	}
}
