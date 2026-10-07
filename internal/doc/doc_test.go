package doc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `---
title: front matter
---
Title
=====
para [link][x]
` + "```go\ncode 1\ncode 2\n```" + `
after

---

[x]: http://example.com

## Section ##

> quote
> more

- a
- b
`

func TestParseBlocks(t *testing.T) {
	d := Parse([]byte(sample))
	want := []struct{ start, end int }{
		{0, 5},   // front matter blanked + setext heading
		{5, 6},   // paragraph
		{6, 10},  // fenced code incl. both fences
		{10, 11}, // "after"
		{12, 13}, // thematic break
		{16, 17}, // heading (ref def at 14 is not a block)
		{18, 20}, // blockquote
		{21, 23}, // list
	}
	if len(d.Blocks) != len(want) {
		for _, b := range d.Blocks {
			t.Logf("%d-%d %q", b.SrcStart, b.SrcEnd, b.Text)
		}
		t.Fatalf("got %d blocks, want %d", len(d.Blocks), len(want))
	}
	for i, w := range want {
		b := d.Blocks[i]
		if b.SrcStart != w.start || b.SrcEnd != w.end {
			t.Errorf("block %d: got %d-%d, want %d-%d (%q)", i, b.SrcStart, b.SrcEnd, w.start, w.end, b.Text)
		}
	}
	if len(d.Headings) != 2 || d.Headings[0].Text != "Title" || d.Headings[1].Text != "Section" || d.Headings[1].Level != 2 {
		t.Errorf("headings: %+v", d.Headings)
	}
	if strings.Contains(d.Blocks[0].Text, "front matter") {
		t.Errorf("front matter leaked into render text: %q", d.Blocks[0].Text)
	}
}

func TestLineMapRoundTrip(t *testing.T) {
	d := Parse([]byte(sample))
	r, err := NewRenderer("notty").Render(d, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "http://example.com") {
		t.Errorf("reference link not resolved:\n%s", strings.Join(r.Lines, "\n"))
	}
	// Every block start must survive source -> rendered -> source.
	for _, b := range d.Blocks {
		if b.SrcStart == 0 {
			continue
		}
		ren := r.SourceToRendered(d, b.SrcStart)
		if got := r.RenderedToSource(d, ren); got != b.SrcStart {
			t.Errorf("block at %d: rendered %d maps back to %d", b.SrcStart, ren, got)
		}
	}
	// Headings land on their rendered text.
	for i, h := range d.Headings {
		line := r.Lines[r.HeadingLine(d, i)]
		if !strings.Contains(line, h.Text) {
			t.Errorf("heading %q rendered line is %q", h.Text, line)
		}
	}
	// Mapping is monotonic.
	prev := -1
	for s := range d.Lines {
		ren := r.SourceToRendered(d, s)
		if ren < prev {
			t.Errorf("source %d -> %d goes backwards (prev %d)", s, ren, prev)
		}
		prev = ren
	}
}

func TestEmpty(t *testing.T) {
	d := Parse(nil)
	r, err := NewRenderer("notty").Render(d, 40)
	if err != nil {
		t.Fatal(err)
	}
	if r.SourceToRendered(d, 0) != 0 || r.RenderedToSource(d, 0) != 0 {
		t.Error("empty doc mapping")
	}
}

func TestLerpInverse(t *testing.T) {
	for from := 1; from < 12; from++ {
		for to := 1; to < 12; to++ {
			for off := range from {
				r := lerp(off, from, to)
				back := lerpUp(r, to, from)
				// back is the first source offset mapping to r, so it maps to r too
				// and is never after off.
				if back > off || lerp(back, from, to) != r {
					t.Fatalf("from=%d to=%d off=%d: r=%d back=%d", from, to, off, r, back)
				}
				if to >= from && back != off {
					t.Fatalf("expanding range not invertible: from=%d to=%d off=%d back=%d", from, to, off, back)
				}
			}
		}
	}
}

func TestLoadRejectsBinary(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(p, []byte("# hi\x00there"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); !errors.Is(err, ErrBinary) {
		t.Fatalf("err = %v, want ErrBinary", err)
	}
}

// A document that opens with a thematic break is not front matter, even
// with another --- further down.
func TestLeadingRuleIsNotFrontMatter(t *testing.T) {
	d := Parse([]byte("---\n\n# Slide one\n\ntext\n\n---\n\n# Slide two\n"))
	if len(d.Headings) != 2 || d.Headings[0].Text != "Slide one" {
		t.Errorf("headings: %+v", d.Headings)
	}
	for _, src := range []string{"---\ntitle: x\n---\n# H\n", "---\n# comment\n---\n# H\n", "---\n---\n# H\n"} {
		d := Parse([]byte(src))
		if len(d.Headings) != 1 || d.Headings[0].Text != "H" || strings.Contains(d.Blocks[0].Text, "---") {
			t.Errorf("%q: front matter not hidden: %+v %q", src, d.Headings, d.Blocks[0].Text)
		}
	}
}

// Standalone bold lines act as one-below-deepest pseudo-headings; bullets
// starting with bold stay out.
func TestStandaloneBoldHeadings(t *testing.T) {
	d := Parse([]byte("### **Title**\n\n**Overview**\n\n- item\n\n**_Case v. Name_, Court (2024)**\n\n- **Facts**\n  - detail\n"))
	if len(d.Headings) != 3 {
		t.Fatalf("headings: %+v", d.Headings)
	}
	if d.Headings[0].Level != 3 || d.Headings[0].Text != "Title" {
		t.Errorf("title: %+v", d.Headings[0])
	}
	for _, h := range d.Headings[1:] {
		if h.Level != 4 {
			t.Errorf("bold level: %+v", h)
		}
	}
	if d.Headings[1].Text != "Overview" || d.Headings[2].Text != "Case v. Name, Court (2024)" {
		t.Errorf("texts: %+v", d.Headings)
	}
}

func TestNoBoldWhenMixedOrAbsent(t *testing.T) {
	d := Parse([]byte("**Bold** and plain\n\n- **lead** item\n\nplain\n"))
	if len(d.Headings) != 0 {
		t.Errorf("headings: %+v", d.Headings)
	}
	d = Parse([]byte("**Only**\n"))
	if len(d.Headings) != 1 || d.Headings[0].Level != 1 || d.Headings[0].Text != "Only" {
		t.Errorf("headings: %+v", d.Headings)
	}
}

// Only the blocks that use a link reference definition get it, matched
// ignoring case and spacing, so other blocks' cached renders survive a
// change to it.
func TestRefsOnlyWhereUsed(t *testing.T) {
	d := Parse([]byte("[some ref]: http://example.com\n\nuses [it][Some  Ref]\n\nplain\n"))
	if !strings.Contains(d.renderText(0), "http://example.com") {
		t.Errorf("block using the ref lacks it: %q", d.renderText(0))
	}
	if d.renderText(1) != d.Blocks[1].Text {
		t.Errorf("block not using the ref got it: %q", d.renderText(1))
	}
}

// Renders at one width don't evict another's, so panes of different widths
// share the cache.
func TestRendererKeepsSeveralWidths(t *testing.T) {
	r := NewRenderer("notty")
	d := Parse([]byte(sample))
	for _, w := range []int{40, 50, 40} {
		if _, err := r.Render(d, w); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.cache[40]) == 0 || len(r.cache[50]) == 0 {
		t.Errorf("cached widths %v", r.widths)
	}
	for w := 60; w < 60+maxWidths; w++ {
		if _, err := r.Render(d, w); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.widths) != maxWidths || len(r.cache) != maxWidths {
		t.Errorf("cache grew past %d widths: %v", maxWidths, r.widths)
	}
}
