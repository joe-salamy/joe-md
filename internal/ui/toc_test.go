package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// tocFixture is a document whose headings are far enough apart that going
// to one scrolls the document:
//
//	0 Intro  1 Setup guide for the project  2 Usage  3 Advanced usage  4 Notes
func tocFixture(t *testing.T) *App {
	t.Helper()
	var sb strings.Builder
	para := strings.Repeat("text\n\n", 30)
	for _, h := range []string{"# Intro", "## Setup guide for the project", "## Usage", "### Advanced usage", "## Notes"} {
		sb.WriteString(h + "\n\n" + para)
	}
	path := filepath.Join(t.TempDir(), "toc.md")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return newTestApp(t, path)
}

// sidebar is the sidebar as drawn, one string per row.
func sidebar(a *App) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(a.View().Content), "\n")[a.tabBarHeight():][:a.bodyHeight()] {
		out = append(out, strings.TrimSpace(ansi.Truncate(l, a.tocWidth(), "")))
	}
	return out
}

func TestTOCFilterNarrows(t *testing.T) {
	a := tocFixture(t)
	press(a, "g", "c")
	if !a.tocFilter.on || a.focus != focusTOC {
		t.Fatal("gc should focus the sidebar and start the filter")
	}
	typeText(a, "USAGE")
	if !slices.Equal(a.tocFilter.shown, []int{2, 3}) {
		t.Fatalf("usage should show Usage and Advanced usage: %v", a.tocFilter.shown)
	}
	if a.toc.cursor != 2 || a.pane.CurrentHeading() != 2 {
		t.Fatalf("the cursor and document should go to the first match: cursor %d, document %d", a.toc.cursor, a.pane.CurrentHeading())
	}
	side := sidebar(a)
	if side[0] != "/ USAGE" || side[1] != "Usage" || side[2] != "Advanced usage" || side[3] != "" {
		t.Fatalf("the sidebar should show the filter and the matches, indented:\n%s", strings.Join(side, "\n"))
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if a.toc.cursor != 3 || a.pane.CurrentHeading() != 3 {
		t.Fatalf("down should move to Advanced usage: cursor %d, document %d", a.toc.cursor, a.pane.CurrentHeading())
	}
	typeText(a, " adv")
	if !slices.Equal(a.tocFilter.shown, []int{3}) || a.toc.cursor != 3 {
		t.Fatalf("every word must match, the cursor staying put: %v, cursor %d", a.tocFilter.shown, a.toc.cursor)
	}
	a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if a.tocFilter.input.Value() != "" || len(a.tocFilter.shown) != 5 || !a.tocFilter.on || a.toc.cursor != 3 {
		t.Fatal("ctrl+c should clear the filter, showing every heading")
	}
	// A click picks the heading on the clicked row of the filtered list.
	typeText(a, "usage")
	a.Update(tea.MouseClickMsg{X: 1, Y: a.tabBarHeight() + tocHeader, Button: tea.MouseLeft})
	if a.toc.cursor != 2 || a.pane.CurrentHeading() != 2 || !a.tocFilter.on {
		t.Fatalf("a click should select Usage and keep filtering: cursor %d", a.toc.cursor)
	}
	a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if a.tocFilter.on || a.focus != focusTOC {
		t.Fatal("ctrl+c on an empty filter should stop it, staying in the sidebar")
	}
}

func TestTOCFilterEnterHidesSidebarAgain(t *testing.T) {
	a := tocFixture(t)
	a.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if a.showTOC {
		t.Fatal("ctrl+t should hide the sidebar")
	}
	press(a, "g", "c")
	if !a.showTOC || a.focus != focusTOC || !a.tocFilter.on {
		t.Fatal("gc should show the sidebar and filter it")
	}
	typeText(a, "notes")
	press(a, "enter")
	if a.showTOC || a.focus != focusDoc || a.tocFilter.on || a.tocFilter.input.Value() != "" {
		t.Fatal("enter should stop filtering and hide the sidebar gc showed")
	}
	if a.pane.CurrentHeading() != 4 {
		t.Fatalf("the document should be at Notes: %d", a.pane.CurrentHeading())
	}

	// With the sidebar already shown, enter leaves it there.
	a.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	press(a, "g", "c")
	typeText(a, "intro")
	press(a, "enter")
	if !a.showTOC || a.focus != focusDoc || a.pane.CurrentHeading() != 0 {
		t.Fatal("enter should keep a sidebar that was already shown")
	}

	// From the sidebar, / filters it too.
	press(a, "tab")
	if a.focus != focusTOC {
		t.Fatal("tab should focus the sidebar")
	}
	press(a, "/")
	if !a.tocFilter.on || a.typing {
		t.Fatal("/ in the sidebar should filter it, not search")
	}
}

func TestTOCFilterEscKeepsSidebar(t *testing.T) {
	a := tocFixture(t)
	a.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	press(a, "g", "c")
	typeText(a, "usage")
	press(a, "esc")
	if a.tocFilter.on || a.tocFilter.input.Value() != "" || a.focus != focusTOC || !a.showTOC || a.toc.cursor != 2 {
		t.Fatalf("esc should clear the filter and stay on Usage in the sidebar: cursor %d", a.toc.cursor)
	}
	side := sidebar(a)
	if side[0] != "Contents" || side[1] != "Intro" || side[5] != "Notes" {
		t.Fatalf("esc should show the full list again:\n%s", strings.Join(side, "\n"))
	}
	press(a, "enter")
	if !a.showTOC || a.focus != focusDoc {
		t.Fatal("after esc the sidebar stays shown")
	}
}

func TestTOCFilterNoMatch(t *testing.T) {
	a := tocFixture(t)
	press(a, "g", "c")
	typeText(a, "usage")
	offset := a.pane.offset
	typeText(a, "zzz")
	if len(a.tocFilter.shown) != 0 || a.toc.cursor != 2 || a.pane.offset != offset {
		t.Fatalf("no match should leave the cursor and document: cursor %d, offset %d (was %d)", a.toc.cursor, a.pane.offset, offset)
	}
	if side := sidebar(a); side[1] != "nothing matches /usagezzz" {
		t.Fatalf("the sidebar should say nothing matches:\n%s", strings.Join(side, "\n"))
	}
	for range 3 {
		a.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if a.tocFilter.input.Value() != "usage" || a.toc.cursor != 2 {
		t.Fatalf("deleting the extra letters should match Usage again: %q, cursor %d", a.tocFilter.input.Value(), a.toc.cursor)
	}
}

// Clicking a pane while filtering ends the filter.
func TestTOCFilterStopsWhenFocusLeaves(t *testing.T) {
	a := tocFixture(t)
	press(a, "g", "c")
	typeText(a, "notes")
	a.Update(tea.MouseClickMsg{X: a.bodyX() + 5, Y: a.tabBarHeight() + 2, Button: tea.MouseLeft})
	if a.tocFilter.on || a.focus != focusDoc {
		t.Fatal("a click on the document should stop the filter")
	}
	_ = a.View()
}
