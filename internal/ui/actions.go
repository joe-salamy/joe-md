package ui

import (
	"path/filepath"

	"github.com/joe-salamy/joe-md/internal/search"

	tea "charm.land/bubbletea/v2"
)

// Every key is bound through the action table below. Each action has a name
// within a context, default keys, and a description; the settings file can
// replace an action's keys by name. The same table drives dispatch, the help
// overlay, -dump-config and KEYS.md.
//
// Contexts:
//
//	normal   the document, and keys that work everywhere outside a pop-up
//	toc      the sidebar, when focused (falls back to normal)
//	results  the results list, when focused (falls back to normal)
//	window   the key after normal.window (ctrl+w)
//	search   the search bar while typing (other keys edit the text)
//	menu     the file menu
//	filter   the file menu's name filter while typing
//	help     the help overlay
//
// A key is spelled as tea's KeyPressMsg.String() spells it. A sequence is
// keys joined by spaces ("g g"); its first keys start the sequence.

// keyCtx names a context of the action table; see above.
type keyCtx string

const (
	ctxNormal  keyCtx = "normal"
	ctxTOC     keyCtx = "toc"
	ctxResults keyCtx = "results"
	ctxWindow  keyCtx = "window"
	ctxSearch  keyCtx = "search"
	ctxMenu    keyCtx = "menu"
	ctxFilter  keyCtx = "filter"
	ctxHelp    keyCtx = "help"
)

// contexts orders the contexts in -dump-config.
var contexts = []keyCtx{ctxNormal, ctxWindow, ctxTOC, ctxResults, ctxSearch, ctxMenu, ctxFilter, ctxHelp}

// call is what an action gets: the count typed before it (0 if none), n (the
// count, at least 1) and the key that triggered it.
type call struct {
	count, n int
	key      string
}

type action struct {
	ctx   keyCtx
	name  string
	group string // help and KEYS.md section
	desc  string
	keys  []string // defaults
	run   func(a *App, c call) tea.Cmd
	// row, if set, merges the action with the ones next to it that have the
	// same row into one line of the help and KEYS.md, described by row.
	row string
}

func keys(k ...string) []string { return k }

// actions is the table; it's filled in init because the run functions refer
// back to App methods that use the keymap.
var actions []*action

// groups orders the help sections.
var groups = []string{
	"Scrolling", "File", "Search", "Search bar", "Results list", "Tabs", "Panes",
	"Focus and layout", "Table of contents", "Quitting", "Help", "File menu", "Menu filter", "Help overlay",
}

// groupNotes follow a group's table in the help and KEYS.md.
var groupNotes = map[string]string{
	"Scrolling":         "`{n}gg` and `{n}G` go to source line *n*.",
	"Results list":      "`{n}gg` and `{n}G` go to result *n*.",
	"Tabs":              "`{n}gt` goes to tab *n*; `3>>` moves the tab three places, stopping at the ends. `tab_n` goes to the tab numbered by the last digit of its key.",
	"Panes":             "These follow the window prefix. A count before the prefix or after it resizes by that much: `5 ctrl+w >` or `ctrl+w 5 >`.",
	"Table of contents": "`{n}gg` and `{n}G` go to heading *n*.",
	"Menu filter":       "Other keys edit the filter, which matches as you type; the search bar's line-editing keys work here too.",
}

// fixedKey is a key that isn't an action, so the settings file can't change
// it: the search bar's line editing, which textinput does itself.
type fixedKey struct{ keys, desc string }

// fixedKeys are listed after a group's actions in the help and KEYS.md.
var fixedKeys = map[string][]fixedKey{
	"Search bar": {
		{"ctrl+w", "delete the word before the cursor"},
		{"ctrl+u/k", "delete to the start / end"},
		{"ctrl+a/e", "go to the start / end"},
		{"alt+b/f", "a word left / right"},
	},
}

func init() {
	half := func(a *App) int { return max(a.bodyHeight()/2, 1) }
	page := func(a *App) int { return max(a.bodyHeight()-2, 1) }
	do := func(f func(a *App, c call)) func(a *App, c call) tea.Cmd {
		return func(a *App, c call) tea.Cmd { f(a, c); return nil }
	}
	// docCmd runs f only when a document is open; docOnly is the same for
	// actions that return no command.
	docCmd := func(f func(a *App, c call) tea.Cmd) func(a *App, c call) tea.Cmd {
		return func(a *App, c call) tea.Cmd {
			if a.pane == nil {
				return nil
			}
			return f(a, c)
		}
	}
	docOnly := func(f func(a *App, c call)) func(a *App, c call) tea.Cmd { return docCmd(do(f)) }
	add := func(ctx keyCtx, group string, list ...*action) {
		for _, x := range list {
			x.ctx, x.group = ctx, group
			actions = append(actions, x)
		}
	}

	add(ctxNormal, "Scrolling",
		&action{name: "scroll_down", desc: "one line down", row: "one line down / up", keys: keys("j", "down"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(c.n) })},
		&action{name: "scroll_up", desc: "one line up", row: "one line down / up", keys: keys("k", "up"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(-c.n) })},
		&action{name: "half_page_down", desc: "half a page down", row: "half a page down / up", keys: keys("ctrl+d"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(c.n * half(a)) })},
		&action{name: "half_page_up", desc: "half a page up", row: "half a page down / up", keys: keys("ctrl+u"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(-c.n * half(a)) })},
		&action{name: "page_down", desc: "a page down", row: "a page down / up", keys: keys("ctrl+f", "pgdown", "space", "f"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(c.n * page(a)) })},
		&action{name: "page_up", desc: "a page up", row: "a page down / up", keys: keys("ctrl+b", "pgup", "shift+space", "b"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(-c.n * page(a)) })},
		&action{name: "top", desc: "top, or source line n", row: "top / bottom, or source line n", keys: keys("g g", "home"),
			run: docOnly(func(a *App, c call) {
				if c.count > 0 {
					a.gotoSourceLine(c.count)
				} else {
					a.pane.ScrollTo(0)
				}
			})},
		&action{name: "bottom", desc: "bottom, or source line n", row: "top / bottom, or source line n", keys: keys("G", "end"),
			run: docOnly(func(a *App, c call) {
				if c.count > 0 {
					a.gotoSourceLine(c.count)
				} else {
					a.pane.ScrollToBottom()
				}
			})},
		&action{name: "next_heading", desc: "next heading", row: "next / previous heading", keys: keys("] ]", "}"),
			run: docOnly(func(a *App, c call) { a.pane.GotoHeading(a.pane.NextHeading(c.n)) })},
		&action{name: "prev_heading", desc: "previous heading", row: "next / previous heading", keys: keys("[ [", "{"),
			run: docOnly(func(a *App, c call) { a.pane.GotoHeading(a.pane.NextHeading(-c.n)) })},
	)
	add(ctxNormal, "File",
		&action{name: "edit", desc: "edit in micro at the top line", keys: keys("e"),
			run: docCmd(func(a *App, c call) tea.Cmd { return a.edit() })},
		&action{name: "reload", desc: "reload from disk", keys: keys("r"),
			run: docCmd(func(a *App, c call) tea.Cmd {
				changed, cmd := a.reload()
				if changed {
					a.msg = "reloaded"
				} else if a.msg == "" {
					a.msg = "unchanged"
				}
				return cmd
			})},
		&action{name: "show_path", desc: "show the file's full path", keys: keys("ctrl+g"),
			run: docOnly(func(a *App, c call) { a.msg = a.pane.doc.Path })},
		&action{name: "copy_name", desc: "copy the file's name", row: "copy the file's name / full path / contents", keys: keys("y n"),
			run: docCmd(func(a *App, c call) tea.Cmd {
				return a.copyText(filepath.Base(a.pane.doc.Path), filepath.Base(a.pane.doc.Path))
			})},
		&action{name: "copy_path", desc: "copy the file's full path", row: "copy the file's name / full path / contents", keys: keys("y p"),
			run: docCmd(func(a *App, c call) tea.Cmd { return a.copyText(a.pane.doc.Path, a.pane.doc.Path) })},
		&action{name: "copy_contents", desc: "copy the file's full contents", row: "copy the file's name / full path / contents", keys: keys("y f"),
			run: docCmd(func(a *App, c call) tea.Cmd { return a.copyText(string(a.pane.doc.Source), "file contents") })},
		&action{name: "open_menu", desc: "open the file menu", keys: keys("o"),
			run: func(a *App, c call) tea.Cmd { return a.menuHere() }},
	)
	add(ctxNormal, "Search",
		&action{name: "search_file", desc: "search this file", keys: keys("/"),
			run: func(a *App, c call) tea.Cmd { return a.startSearch(search.File) }},
		&action{name: "search_open", desc: "search the open files", keys: keys("g b"),
			run: func(a *App, c call) tea.Cmd { return a.startSearch(search.Open) }},
		&action{name: "search_files", desc: "search the directory or repo", keys: keys("?"),
			run: func(a *App, c call) tea.Cmd { return a.startSearch(a.crossScope) }},
		&action{name: "next_match", desc: "next match", row: "next / previous match", keys: keys("n"),
			run: docOnly(func(a *App, c call) { a.nextMatch(c.n) })},
		&action{name: "prev_match", desc: "previous match", row: "next / previous match", keys: keys("N"),
			run: docOnly(func(a *App, c call) { a.nextMatch(-c.n) })},
		&action{name: "toggle_results", desc: "open / close the results list", keys: keys("g r"),
			run: do(func(a *App, c call) { a.toggleResults() })},
		&action{name: "next_result", desc: "next result, in the focused pane", row: "next / previous result, in the focused pane", keys: keys("] q"),
			run: docOnly(func(a *App, c call) { a.stepResult(c.n) })},
		&action{name: "prev_result", desc: "previous result, in the focused pane", row: "next / previous result, in the focused pane", keys: keys("[ q"),
			run: docOnly(func(a *App, c call) { a.stepResult(-c.n) })},
		&action{name: "clear_search", desc: "kill the search: no more highlights or results", keys: keys("esc"),
			run: do(func(a *App, c call) { a.killSearch() })},
		&action{name: "toggle_search_bar", desc: "show / hide the search bar", keys: keys("alt+s"),
			run: do(func(a *App, c call) { a.showBar = !a.showBar; a.layout() })},
	)
	add(ctxSearch, "Search bar",
		&action{name: "submit", desc: "search (empty repeats the last search)", keys: keys("enter"),
			run: func(a *App, c call) tea.Cmd { return a.submit() }},
		&action{name: "cancel", desc: "cancel and kill the search", keys: keys("esc"),
			run: do(func(a *App, c call) { a.stopTyping(); a.killSearch() })},
		&action{name: "clear", desc: "clear the input (again to cancel)", keys: keys("ctrl+c"),
			run: do(func(a *App, c call) {
				if a.input.Value() != "" {
					a.input.SetValue("")
				} else { // same as cancel
					a.stopTyping()
					a.killSearch()
				}
			})},
		&action{name: "scope_next", desc: "next scope: file → open → dir → repo", row: "next / previous scope: file → open → dir → repo", keys: keys("tab"),
			run: do(func(a *App, c call) { a.scope = a.scope.Next(1) })},
		&action{name: "scope_prev", desc: "previous scope", row: "next / previous scope: file → open → dir → repo", keys: keys("shift+tab"),
			run: do(func(a *App, c call) { a.scope = a.scope.Next(-1) })},
		&action{name: "toggle_literal", desc: "regex / literal text", keys: keys("ctrl+r"),
			run: do(func(a *App, c call) { a.mode.Literal = !a.mode.Literal; a.setPlaceholder() })},
		&action{name: "history_prev", desc: "previous search in history", row: "previous / next search in history", keys: keys("up"),
			run: do(func(a *App, c call) { a.historyStep(-1) })},
		&action{name: "history_next", desc: "next search in history", row: "previous / next search in history", keys: keys("down"),
			run: do(func(a *App, c call) { a.historyStep(1) })},
	)

	rsel := func(f func(r *Results, c call, rows int) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { r, rows := a.results, a.resultRows(); r.Select(f(r, c, rows), rows) })
	}
	add(ctxResults, "Results list",
		&action{name: "down", desc: "next result", row: "next / previous result", keys: keys("j", "down"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor + c.n })},
		&action{name: "up", desc: "previous result", row: "next / previous result", keys: keys("k", "up"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor - c.n })},
		&action{name: "half_down", desc: "half a page down", row: "half a page down / up", keys: keys("ctrl+d"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor + c.n*max(rows/2, 1) })},
		&action{name: "half_up", desc: "half a page up", row: "half a page down / up", keys: keys("ctrl+u"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor - c.n*max(rows/2, 1) })},
		&action{name: "first", desc: "first result, or result n", row: "first / last result, or result n", keys: keys("g g", "home"),
			run: rsel(func(r *Results, c call, rows int) int { return c.n - 1 })},
		&action{name: "last", desc: "last result, or result n", row: "first / last result, or result n", keys: keys("G", "end"),
			run: rsel(func(r *Results, c call, rows int) int {
				if c.count > 0 {
					return c.count - 1
				}
				return len(r.res.Matches) - 1
			})},
		&action{name: "open", desc: "open in a new tab", keys: keys("enter", "l"),
			run: do(func(a *App, c call) { a.openResult(openNewTab) })},
		&action{name: "open_here", desc: "open in the focused pane", keys: keys("O"),
			run: do(func(a *App, c call) { a.openResult(openReplace) })},
		&action{name: "open_vsplit", desc: "open in a new pane beside", row: "open in a new pane beside / below", keys: keys("v"),
			run: do(func(a *App, c call) { a.openResult(openVSplit) })},
		&action{name: "open_hsplit", desc: "open in a new pane below", row: "open in a new pane beside / below", keys: keys("s"),
			run: do(func(a *App, c call) { a.openResult(openHSplit) })},
		&action{name: "close", desc: "close the list, killing the search too", keys: keys("q", "esc"),
			run: do(func(a *App, c call) { a.killSearch() })},
	)

	add(ctxNormal, "Tabs",
		&action{name: "next_tab", desc: "next tab, or tab n", row: "next / previous tab, or tab n", keys: keys("g t"),
			run: do(func(a *App, c call) {
				if c.count > 0 {
					a.gotoTab(c.count)
				} else {
					a.stepTab(1)
				}
			})},
		&action{name: "prev_tab", desc: "previous tab", row: "next / previous tab, or tab n", keys: keys("g T"),
			run: do(func(a *App, c call) { a.stepTab(-c.n) })},
		&action{name: "tab_n", desc: "tab 1 … 9", keys: keys("alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9"),
			run: do(func(a *App, c call) {
				if d := c.key[len(c.key)-1]; d >= '1' && d <= '9' {
					a.gotoTab(int(d - '0'))
				}
			})},
		&action{name: "move_tab_left", desc: "move the tab n places left", row: "move the tab n places left / right", keys: keys("< <"),
			run: do(func(a *App, c call) { a.moveTab(-c.n) })},
		&action{name: "move_tab_right", desc: "move the tab n places right", row: "move the tab n places left / right", keys: keys("> >"),
			run: do(func(a *App, c call) { a.moveTab(c.n) })},
		&action{name: "close_tab", desc: "close the tab (not the last one)", keys: keys("x"),
			run: do(func(a *App, c call) {
				if len(a.tabs) == 1 {
					a.msg = "last tab (q quits)"
				} else {
					a.closeTab()
				}
			})},
		&action{name: "reopen", desc: "reopen the last closed tab or pane where it was, or the last n", keys: keys("X"),
			run: func(a *App, c call) tea.Cmd { return a.reopen(c.n) }},
		&action{name: "toggle_tab_bar", desc: "show / hide the tab bar", keys: keys("alt+t"),
			run: do(func(a *App, c call) { a.showTabs = !a.showTabs; a.layout() })},
	)

	add(ctxNormal, "Panes",
		&action{name: "window", desc: "prefix for the pane keys below", keys: keys("ctrl+w"),
			run: do(func(a *App, c call) { a.window = true; a.count = c.count })},
	)
	resize := func(vert bool, sign int) func(a *App, c call) tea.Cmd {
		return docOnly(func(a *App, c call) {
			if a.tab().resize(vert, sign*c.n) {
				a.layout()
			}
		})
	}
	add(ctxWindow, "Panes",
		&action{name: "split_vertical", desc: "split side by side", row: "split side by side / stacked", keys: keys("v"),
			run: docOnly(func(a *App, c call) { a.splitPane(true) })},
		&action{name: "split_horizontal", desc: "split stacked", row: "split side by side / stacked", keys: keys("s"),
			run: docOnly(func(a *App, c call) { a.splitPane(false) })},
		&action{name: "focus_left", desc: "focus the pane left (past the edge: the sidebar)", row: "focus the pane left / below / above / right (past the edge: sidebar / results)", keys: keys("h"),
			run: docOnly(func(a *App, c call) { a.moveFocus("h") })},
		&action{name: "focus_down", desc: "focus the pane below (past the edge: the results)", row: "focus the pane left / below / above / right (past the edge: sidebar / results)", keys: keys("j"),
			run: docOnly(func(a *App, c call) { a.moveFocus("j") })},
		&action{name: "focus_up", desc: "focus the pane above", row: "focus the pane left / below / above / right (past the edge: sidebar / results)", keys: keys("k"),
			run: docOnly(func(a *App, c call) { a.moveFocus("k") })},
		&action{name: "focus_right", desc: "focus the pane right", row: "focus the pane left / below / above / right (past the edge: sidebar / results)", keys: keys("l"),
			run: docOnly(func(a *App, c call) { a.moveFocus("l") })},
		&action{name: "next_pane", desc: "next pane, then results and sidebar", row: "next / previous pane, then results and sidebar", keys: keys("w"),
			run: docOnly(func(a *App, c call) { a.cycleFocus(c.n) })},
		&action{name: "prev_pane", desc: "previous pane", row: "next / previous pane, then results and sidebar", keys: keys("W"),
			run: docOnly(func(a *App, c call) { a.cycleFocus(-c.n) })},
		&action{name: "only", desc: "close every other pane", keys: keys("o"),
			run: docOnly(func(a *App, c call) { a.closeOthers() })},
		&action{name: "equalize", desc: "make all panes the same size", keys: keys("="),
			run: docOnly(func(a *App, c call) { a.tab().root.equalize(); a.layout() })},
		&action{name: "wider", desc: "n columns wider", row: "n columns wider / narrower", keys: keys(">"), run: resize(true, 1)},
		&action{name: "narrower", desc: "n columns narrower", row: "n columns wider / narrower", keys: keys("<"), run: resize(true, -1)},
		&action{name: "taller", desc: "n rows taller", row: "n rows taller / shorter", keys: keys("+"), run: resize(false, 1)},
		&action{name: "shorter", desc: "n rows shorter", row: "n rows taller / shorter", keys: keys("-"), run: resize(false, -1)},
		&action{name: "scrollbind", desc: "scrollbind on / off for the pane", keys: keys("b"),
			run: docOnly(func(a *App, c call) {
				a.pane.bind = !a.pane.bind
				a.msg = "scrollbind off"
				if a.pane.bind {
					a.msg = "scrollbind on"
				}
			})},
	)

	add(ctxNormal, "Focus and layout",
		&action{name: "focus_next", desc: "cycle focus: sidebar, panes, results", row: "cycle focus: sidebar, panes, results / backwards", keys: keys("tab"),
			run: docOnly(func(a *App, c call) { a.cycleFocus(1) })},
		&action{name: "focus_prev", desc: "cycle focus backwards", row: "cycle focus: sidebar, panes, results / backwards", keys: keys("shift+tab"),
			run: docOnly(func(a *App, c call) { a.cycleFocus(-1) })},
		&action{name: "toggle_toc", desc: "show / hide the table of contents", keys: keys("ctrl+t"),
			run: do(func(a *App, c call) {
				a.showTOC = !a.showTOC
				if !a.showTOC && a.focus == focusTOC {
					a.focus = focusDoc
				}
				a.layout()
			})},
	)

	tsel := func(f func(a *App, c call) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { a.tocSelect(f(a, c)) })
	}
	add(ctxTOC, "Table of contents",
		&action{name: "down", desc: "next heading (the document follows)", row: "next / previous heading (the document follows)", keys: keys("j", "down"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor + c.n })},
		&action{name: "up", desc: "previous heading", row: "next / previous heading (the document follows)", keys: keys("k", "up"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor - c.n })},
		&action{name: "half_down", desc: "half a page down", row: "half a page down / up", keys: keys("ctrl+d"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor + c.n*half(a) })},
		&action{name: "half_up", desc: "half a page up", row: "half a page down / up", keys: keys("ctrl+u"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor - c.n*half(a) })},
		&action{name: "first", desc: "first heading, or heading n", row: "first / last heading, or heading n", keys: keys("g g", "home"),
			run: tsel(func(a *App, c call) int { return c.n - 1 })},
		&action{name: "last", desc: "last heading, or heading n", row: "first / last heading, or heading n", keys: keys("G", "end"),
			run: tsel(func(a *App, c call) int {
				if c.count > 0 {
					return c.count - 1
				}
				return len(a.pane.doc.Headings) - 1
			})},
		&action{name: "back", desc: "back to the document", keys: keys("enter", "l", "esc", "h"),
			run: do(func(a *App, c call) { a.focus = focusDoc })},
	)

	add(ctxNormal, "Quitting",
		&action{name: "quit", desc: "kill the search if there is one, else close the pane, then the tab, then quit", keys: keys("q"),
			run: func(a *App, c call) tea.Cmd {
				if a.searchActive() {
					a.killSearch()
					return nil
				}
				if !a.closePane() {
					return tea.Quit
				}
				return nil
			}},
		&action{name: "quit_all", desc: "quit", keys: keys("Q", "ctrl+c"),
			run: func(a *App, c call) tea.Cmd { return tea.Quit }},
	)
	add(ctxNormal, "Help",
		&action{name: "help", desc: "show the keys", keys: keys("f1", "g ?"),
			run: do(func(a *App, c call) { a.help = &Help{} })},
	)

	msel := func(f func(m *Menu, c call, rows int) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { m, rows := a.menu, a.menuRows(); m.move(f(m, c, rows), rows) })
	}
	mselKeep := func(f func(m *Menu, c call, rows int) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { m, rows := a.menu, a.menuRows(); m.moveKeep(f(m, c, rows), rows) })
	}
	mselExt := func(f func(m *Menu, c call, rows int) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { m, rows := a.menu, a.menuRows(); m.extend(f(m, c, rows), rows) })
	}
	add(ctxMenu, "File menu",
		&action{name: "down", desc: "next entry", row: "next / previous entry", keys: keys("j", "down"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor + c.n })},
		&action{name: "up", desc: "previous entry", row: "next / previous entry", keys: keys("k", "up"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor - c.n })},
		&action{name: "half_down", desc: "half a page down", row: "half a page down / up", keys: keys("ctrl+d", "pgdown"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor + c.n*max(rows/2, 1) })},
		&action{name: "half_up", desc: "half a page up", row: "half a page down / up", keys: keys("ctrl+u", "pgup"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor - c.n*max(rows/2, 1) })},
		&action{name: "first", desc: "first entry", row: "first / last entry", keys: keys("g g", "home"),
			run: msel(func(m *Menu, c call, rows int) int { return 0 })},
		&action{name: "last", desc: "last entry", row: "first / last entry", keys: keys("G", "end"),
			run: msel(func(m *Menu, c call, rows int) int { return len(m.shown) - 1 })},
		&action{name: "down_extend", desc: "mark to the next entry", row: "mark to the next / previous entry", keys: keys("shift+j", "shift+down", "J"),
			run: mselExt(func(m *Menu, c call, rows int) int { return m.cursor + c.n })},
		&action{name: "up_extend", desc: "mark to the previous entry", row: "mark to the next / previous entry", keys: keys("shift+k", "shift+up", "K"),
			run: mselExt(func(m *Menu, c call, rows int) int { return m.cursor - c.n })},
		&action{name: "down_keep", desc: "next entry, keeping the marks", row: "next / previous entry, keeping the marks", keys: keys("ctrl+j", "ctrl+n", "ctrl+down"),
			run: mselKeep(func(m *Menu, c call, rows int) int { return m.cursor + c.n })},
		&action{name: "up_keep", desc: "previous entry, keeping the marks", row: "next / previous entry, keeping the marks", keys: keys("ctrl+k", "ctrl+p", "ctrl+up"),
			run: mselKeep(func(m *Menu, c call, rows int) int { return m.cursor - c.n })},
		&action{name: "open", desc: "enter the directory, or open the marked files in new tabs", keys: keys("l", "right", "enter"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openNewTab) }},
		&action{name: "toggle", desc: "mark / unmark the file", keys: keys("space", "t"),
			run: do(func(a *App, c call) { a.menu.flip(); a.menu.moveKeep(a.menu.cursor+1, a.menuRows()) })},
		&action{name: "toggle_here", desc: "mark / unmark the file, staying put", keys: keys("ctrl+space", "ctrl+t"),
			run: do(func(a *App, c call) { a.menu.flip() })},
		&action{name: "select_all", desc: "mark / unmark every shown file", keys: keys("ctrl+a"),
			run: do(func(a *App, c call) { a.menu.markAll() })},
		&action{name: "open_here", desc: "open the file in the focused pane", keys: keys("O"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openReplace) }},
		&action{name: "open_vsplit", desc: "open the file in a new pane beside", row: "open the file in a new pane beside / below", keys: keys("v"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openVSplit) }},
		&action{name: "open_hsplit", desc: "open the file in a new pane below", row: "open the file in a new pane beside / below", keys: keys("s"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openHSplit) }},
		&action{name: "parent", desc: "parent directory", keys: keys("h", "left", "-", "backspace"),
			run: do(func(a *App, c call) { a.menuUp() })},
		&action{name: "home", desc: "home directory", keys: keys("~", "g h"),
			run: do(func(a *App, c call) { a.menuHome() })},
		&action{name: "repo_root", desc: "root of the git repository", keys: keys("g r"),
			run: do(func(a *App, c call) {
				if root := search.RepoRoot(a.menu.dir); root != "" {
					a.menuLoad(root, "")
				} else {
					a.menu.err = "not in a git repository"
				}
			})},
		&action{name: "toggle_all", desc: "show / hide hidden, ignored and non-markdown files", keys: keys("."),
			run: do(func(a *App, c call) { a.menuAll = !a.menuAll; a.menu.refilter(a.menuAll, a.menuRows()) })},
		&action{name: "refresh", desc: "re-read the directory", keys: keys("r"),
			run: do(func(a *App, c call) {
				sel := ""
				if e, ok := a.menu.current(); ok {
					sel = e.name
				}
				a.menuLoad(a.menu.dir, sel)
			})},
		&action{name: "filter", desc: "filter names", keys: keys("/"),
			run: func(a *App, c call) tea.Cmd { a.menu.filtering = true; return a.menu.filter.Focus() }},
		&action{name: "close", desc: "close the menu (esc clears the marks first)", keys: keys("esc", "q", "o", "ctrl+c"),
			run: func(a *App, c call) tea.Cmd {
				if c.key == "esc" && a.menu.filter.Value() != "" {
					a.menu.filter.SetValue("")
					a.menu.refilter(a.menuAll, a.menuRows())
					return nil
				}
				if c.key == "esc" && len(a.menu.sel) > 0 {
					a.menu.clearSel()
					a.menu.anchor = a.menu.cursor
					return nil
				}
				return a.closeMenu()
			}},
		&action{name: "quit", desc: "quit", keys: keys("Q"),
			run: func(a *App, c call) tea.Cmd { return tea.Quit }},
	)
	add(ctxFilter, "Menu filter",
		&action{name: "accept", desc: "stop filtering and open the selection", keys: keys("enter"),
			run: func(a *App, c call) tea.Cmd { a.menu.stopFilter(); return a.menuActivate(openNewTab) }},
		&action{name: "cancel", desc: "clear the filter and stop", keys: keys("esc"),
			run: do(func(a *App, c call) { a.menu.stopFilter(); a.menu.setFilter("", a.menuAll, a.menuRows()) })},
		&action{name: "clear", desc: "clear the filter (again to stop)", keys: keys("ctrl+c"),
			run: do(func(a *App, c call) {
				if a.menu.filter.Value() == "" {
					a.menu.stopFilter()
				} else {
					a.menu.setFilter("", a.menuAll, a.menuRows())
				}
			})},
		&action{name: "down", desc: "next entry", row: "next / previous entry", keys: keys("down"),
			run: mselKeep(func(m *Menu, c call, rows int) int { return m.cursor + 1 })},
		&action{name: "up", desc: "previous entry", row: "next / previous entry", keys: keys("up"),
			run: mselKeep(func(m *Menu, c call, rows int) int { return m.cursor - 1 })},
	)

	hscroll := func(f func(a *App, c call) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { a.help.scroll(f(a, c), len(a.helpLines()), a.helpRows()) })
	}
	add(ctxHelp, "Help overlay",
		&action{name: "down", desc: "scroll down", row: "scroll down / up", keys: keys("j", "down"),
			run: hscroll(func(a *App, c call) int { return c.n })},
		&action{name: "up", desc: "scroll up", row: "scroll down / up", keys: keys("k", "up"),
			run: hscroll(func(a *App, c call) int { return -c.n })},
		&action{name: "page_down", desc: "a page down", row: "a page down / up", keys: keys("ctrl+d", "ctrl+f", "pgdown", "space"),
			run: hscroll(func(a *App, c call) int { return c.n * max(a.helpRows()-1, 1) })},
		&action{name: "page_up", desc: "a page up", row: "a page down / up", keys: keys("ctrl+u", "ctrl+b", "pgup", "b"),
			run: hscroll(func(a *App, c call) int { return -c.n * max(a.helpRows()-1, 1) })},
		&action{name: "top", desc: "top", row: "top / bottom", keys: keys("g g", "home"),
			run: hscroll(func(a *App, c call) int { return -len(a.helpLines()) })},
		&action{name: "bottom", desc: "bottom", row: "top / bottom", keys: keys("G", "end"),
			run: hscroll(func(a *App, c call) int { return len(a.helpLines()) })},
		&action{name: "close", desc: "close the help", keys: keys("esc", "q", "f1", "g ?"),
			run: do(func(a *App, c call) { a.help = nil })},
	)
}
