package doc

import (
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
