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
