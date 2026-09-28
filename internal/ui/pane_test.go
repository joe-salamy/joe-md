package ui

import (
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/doc"

	"github.com/charmbracelet/x/ansi"
)

// A pane wider than the wrap centres the document; a line too wide for the
// margin shifts left instead of being cut off.
func TestPaneCentres(t *testing.T) {
	p := &Pane{
		doc:    &doc.Doc{},
		view:   &doc.Rendered{Lines: []string{"abcd", strings.Repeat("x", 18)}},
		width:  20,
		height: 2,
		wrap:   10,
	}
	got := p.Render(theme{})
	for i, want := range []string{"     abcd", "  " + strings.Repeat("x", 18)} {
		if s := strings.TrimRight(ansi.Strip(got[i]), " "); s != want {
			t.Errorf("line %d = %q, want %q", i, s, want)
		}
	}
	p.width = 10 // no spare room: no margin
	if s := ansi.Strip(p.Render(theme{})[0]); !strings.HasPrefix(s, "abcd") {
		t.Errorf("narrow line = %q, want no margin", s)
	}
}

// A jump to a heading near the end stops at the end, and that heading is
// still the current one, so ]] and [[ step on from it.
func TestHeadingJumpStopsAtEnd(t *testing.T) {
	var sb strings.Builder
	for i := range 4 {
		sb.WriteString("# H" + string(rune('0'+i)) + "\n\n" + strings.Repeat("text\n\n", 6))
	}
	sb.WriteString("# Last\n\nend\n")
	p := testPane(t, sb.String())
	last := len(p.doc.Headings) - 1

	p.GotoHeading(3)
	if p.offset != p.view.HeadingLine(p.doc, 3) {
		t.Fatalf("H3: offset %d, want it at the top", p.offset)
	}
	p.GotoHeading(p.NextHeading(1))
	if p.offset != p.maxOffset() || p.CurrentHeading() != last {
		t.Fatalf("Last: offset %d max %d current %d", p.offset, p.maxOffset(), p.CurrentHeading())
	}
	if p.NextHeading(1) != last || p.NextHeading(-1) != 3 {
		t.Fatalf("from Last: ]] %d [[ %d", p.NextHeading(1), p.NextHeading(-1))
	}
	// Scrolling away drops the pin.
	p.ScrollBy(-1)
	if p.CurrentHeading() != 3 {
		t.Fatalf("after scrolling: current %d", p.CurrentHeading())
	}
}
