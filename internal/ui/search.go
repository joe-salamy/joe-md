package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/search"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The search bar sits above the status line. ripgrep only runs when enter is
// pressed, never while typing, and always in the background: the UI stays
// responsive and a newer search cancels an older one.

const maxHistory = 100

type searchPurpose int

const (
	searchInteractive searchPurpose = iota // typed by the user: jump or list
	searchRefresh                          // re-run after a reload: just re-highlight
)

type searchDoneMsg struct {
	seq     int
	pane    *Pane // the pane a file search is for
	req     search.Request
	purpose searchPurpose
	res     search.Result
	pre     *matches // pane's matches, indexed in the background
	err     error
}

// newInput is a line editor for the search bar or the menu filter.
func newInput(dark bool, placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.SetStyles(textinput.DefaultStyles(dark))
	return ti
}

// modeName describes how queries match, for the search bar. The case is
// named as the settings file names it.
func modeName(m search.Mode) string {
	s := "regex"
	if m.Literal {
		s = "literal"
	}
	return s + ", case: " + m.Case.String()
}

func (a *App) setPlaceholder() {
	h := func(name string) string { return a.hint(ctxSearch, name) }
	a.input.Placeholder = modeName(a.mode) + " · " + h("scope_next") + ": scope · " +
		h("toggle_literal") + ": regex/literal · " + h("submit") + ": search"
}

// startSearch opens the search bar for typing, starting in scope s.
func (a *App) startSearch(s search.Scope) tea.Cmd {
	a.scope = s
	a.typing = true
	a.histIdx = len(a.history)
	a.input.SetValue("")
	a.layout() // the bar may have been hidden
	return a.input.Focus()
}

func (a *App) stopTyping() {
	a.typing = false
	a.input.Blur()
	a.layout()
}

// inputKey handles a key while typing in the search bar. Keys that aren't
// bound in the search context edit the text, with textinput's line editing
// (ctrl+w, ctrl+u, ctrl+a/e, ctrl+h, ctrl+k, alt+b/f, ...).
func (a *App) inputKey(msg tea.KeyPressMsg) tea.Cmd {
	var none string
	if cmd, ok := a.dispatch(&none, msg.String(), 0, ctxSearch); ok {
		return cmd
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return cmd
}

// historyStep recalls older (d < 0) or newer searches. Stepping past the
// newest one restores what was being typed.
func (a *App) historyStep(d int) {
	if len(a.history) == 0 {
		return
	}
	if a.histIdx == len(a.history) {
		a.draft = a.input.Value()
	}
	a.histIdx = max(0, min(a.histIdx+d, len(a.history)))
	if a.histIdx == len(a.history) {
		a.input.SetValue(a.draft)
	} else {
		a.input.SetValue(a.history[a.histIdx])
	}
	a.input.CursorEnd()
}

// submit runs the typed search. Like vim, an empty query repeats the last one.
func (a *App) submit() tea.Cmd {
	q := a.input.Value()
	if q == "" && len(a.history) > 0 {
		q = a.history[len(a.history)-1]
	}
	a.stopTyping()
	if q == "" {
		return nil
	}
	if i := slices.Index(a.history, q); i >= 0 {
		a.history = slices.Delete(a.history, i, i+1)
	}
	a.history = append(a.history, q)
	if len(a.history) > maxHistory {
		a.history = a.history[1:]
	}
	if a.scope != search.File {
		a.crossScope = a.scope
	}
	req := search.Request{Query: q, Mode: a.mode, Scope: a.scope, Root: search.Root(a.pane.doc.Path, a.scope)}
	if a.scope == search.Open {
		req.Files = a.openFiles()
		if len(req.Files) == 1 {
			req.Root = req.Files[0]
		}
	}
	return a.runSearch(req, a.pane)
}

// openFiles is every open file, deduplicated, in tab order: what the open
// scope searches. Paths are absolute (see doc.Load), so they compare with ==.
func (a *App) openFiles() []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range a.allPanes() {
		if path := p.doc.Path; !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

// runSearch starts a typed search in the background, cancelling any search
// still running. p is the pane it was started from.
func (a *App) runSearch(req search.Request, p *Pane) tea.Cmd {
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.seq++
	seq := a.seq
	a.searching = req.Query
	return searchCmd(ctx, searchDoneMsg{seq: seq, pane: p, req: req, purpose: searchInteractive})
}

// searchCmd runs msg.req in the background and returns msg with the result,
// and the matches in msg.pane's file indexed against its current rendering.
func searchCmd(ctx context.Context, msg searchDoneMsg) tea.Cmd {
	d, v := msg.pane.doc, msg.pane.view
	return func() tea.Msg {
		msg.res, msg.err = search.Run(ctx, msg.req)
		if msg.err == nil {
			msg.pre = indexed(msg.req, msg.res.Matches, d, v)
		}
		return msg
	}
}

// indexed returns the matches in d's file, indexed against rendering v. It
// runs in the background: placing thousands of matches takes long enough to
// stall the UI.
func indexed(req search.Request, ms []search.Match, d *doc.Doc, v *doc.Rendered) *matches {
	if req.Scope != search.File {
		var in []search.Match
		for _, m := range ms {
			if m.Path == d.Path {
				in = append(in, m)
			}
		}
		ms = in
	}
	m := newMatches(req, ms)
	m.index(d, v)
	return m
}

// refreshSearch re-runs pane p's search after its file changed, so the
// highlights follow the new text. Refreshes run alongside typed searches and
// each other: several panes may need one at once.
func (a *App) refreshSearch(p *Pane) tea.Cmd {
	q := p.Query()
	if q == "" {
		return nil
	}
	req := search.Request{Query: q, Mode: p.match.mode, Scope: search.File, Root: search.Root(p.doc.Path, search.File)}
	return searchCmd(context.Background(), searchDoneMsg{pane: p, req: req, purpose: searchRefresh})
}

func (a *App) searchDone(m searchDoneMsg) {
	if m.purpose == searchRefresh {
		if m.err == nil && a.livePane(m.pane) && m.pane.Query() == m.req.Query {
			m.pane.setMatches(m.pre)
		}
		return
	}
	if m.seq != a.seq {
		return // superseded
	}
	a.cancel, a.searching = nil, ""
	if m.err != nil {
		a.msg = "search: " + m.err.Error()
		return
	}
	q, matches := m.req.Query, m.res.Matches
	// A file search belongs to the pane it was started in, which may no
	// longer have focus (or be open at all).
	p := m.pane
	if m.req.Scope == search.File && !a.livePane(p) {
		return
	}
	switch {
	case m.req.Scope == search.File && p != a.pane:
		p.setMatches(m.pre) // focus moved meanwhile: no jump

	case m.req.Scope == search.File:
		// Vim-style: jump to the first match below the view; gr lists them.
		a.setResults(m.req, m.res)
		p.setMatches(m.pre)
		if len(matches) == 0 {
			a.msg = "no matches for " + q
			return
		}
		wrapped, _ := a.pane.JumpMatch(1, true)
		a.reportJump(1, wrapped)
		a.syncResults()

	default:
		if len(matches) == 0 {
			a.msg = "no matches for " + q + " in " + m.req.Scope.String()
			return
		}
		a.setResults(m.req, m.res)
		a.showResults = true
		a.focus = focusResults
		a.layout()
		if a.pane == p {
			p.setMatches(m.pre)
		} else {
			a.pane.setMatches(newMatches(m.req, a.results.InFile(a.pane.doc.Path)))
		}
		a.msg = plural(a.results.Len(), "match", "matches") + " in " + plural(m.res.Files, "file", "files")
		if m.res.Truncated {
			a.msg += " (limit reached)"
		}
	}
}

func (a *App) setResults(req search.Request, res search.Result) {
	a.results = newResults(req, res)
	if len(res.Matches) == 0 {
		a.showResults = false
		if a.focus == focusResults {
			a.focus = focusDoc
		}
	}
	a.layout()
}

// nextMatch is n / N.
func (a *App) nextMatch(n int) {
	if a.pane.Query() == "" {
		a.msg = "no previous search"
		return
	}
	wrapped, ok := a.pane.JumpMatch(n, false)
	if !ok {
		a.msg = "no matches for " + a.pane.Query()
		return
	}
	a.reportJump(n, wrapped)
	a.syncResults()
}

// syncResults points the results list at the pane's current match, if the
// list is from the same search.
func (a *App) syncResults() {
	p, r := a.pane, a.results
	if p == nil || p.match == nil || p.match.cur < 0 || r == nil ||
		r.req.Query != p.match.query || r.req.Mode != p.match.mode {
		return
	}
	src, nth, _ := p.CurrentMatch()
	if i := r.Find(p.doc.Path, src+1, nth); i >= 0 {
		r.Select(i, a.resultRows())
	}
}

// killSearch is esc: the running search stops, the results list is dropped,
// and no pane highlights matches any more. Killed means killed: bumping the
// sequence drops a search that is still computing, so its matches can never
// land after the highlights are gone.
// searchActive reports whether there is a search for killSearch to kill: one
// running, a results list, or highlights in any pane.
func (a *App) searchActive() bool {
	if a.cancel != nil || a.results != nil {
		return true
	}
	for _, p := range a.allPanes() {
		if p.match != nil {
			return true
		}
	}
	return false
}

func (a *App) killSearch() {
	if a.cancel != nil {
		a.cancel()
		a.cancel, a.searching = nil, ""
		a.seq++ // the killed search's completion is stale: ignore it
	}
	for _, p := range a.allPanes() {
		p.ClearMatches()
	}
	a.closeResults()
	a.results = nil
}

func (a *App) reportJump(n int, wrapped bool) {
	switch {
	case !wrapped:
	case n > 0:
		a.msg = "search hit BOTTOM, continuing at TOP"
	default:
		a.msg = "search hit TOP, continuing at BOTTOM"
	}
}

// openResult shows the selected result, with the rest of that file's matches
// highlighted for n / N.
func (a *App) openResult(mode openMode) {
	m, nth := a.results.Current()
	if !a.openAt(m.Path, mode) {
		return
	}
	a.pane.setMatches(newMatches(a.results.req, a.results.InFile(m.Path)))
	a.pane.GotoMatch(m.Line-1, nth)
	a.focus = focusDoc
	a.msg = relPath(a.results.req, m.Path) + ":" + strconv.Itoa(m.Line) +
		"  (" + strconv.Itoa(a.results.cursor+1) + "/" + strconv.Itoa(a.results.Len()) + ")"
}

// stepResult is ]q / [q: open the next or previous result from anywhere.
func (a *App) stepResult(n int) {
	if a.results == nil || len(a.results.res.Matches) == 0 {
		a.msg = "no search results"
		return
	}
	a.results.Select(a.results.cursor+n, a.resultRows())
	a.openResult(openReplace) // like vim's quickfix, reuse the pane
}

func (a *App) resultRows() int { return max(a.panelHeight()-1, 1) }

// toggleResults is gr: open and focus the results panel, or close it.
func (a *App) toggleResults() {
	switch {
	case a.results == nil || len(a.results.res.Matches) == 0:
		a.msg = "no search results"
	case a.showResults && a.focus == focusResults:
		a.closeResults()
	default:
		a.showResults = true
		a.focus = focusResults
		a.layout()
	}
}

func (a *App) closeResults() {
	a.showResults = false
	if a.focus == focusResults {
		a.focus = focusDoc
	}
	a.layout()
}

func (a *App) barHeight() int {
	if a.showBar || a.typing {
		return 1
	}
	return 0
}

func (a *App) panelHeight() int {
	if !a.showResults || a.results == nil {
		return 0
	}
	return a.results.Height(a.height)
}

// barView is the one-line search bar.
func (a *App) barView() string {
	th, w := a.theme, a.width
	if a.typing {
		left, right := a.typingBar()
		line := left + a.input.View()
		gap := w - ansi.StringWidth(line) - ansi.StringWidth(right)
		return fit(line+strings.Repeat(" ", max(gap, 0))+right, w)
	}
	if a.searching != "" {
		return fit(th.barDim.Render(" searching for ")+th.bar.Render(a.searching)+th.barDim.Render(" …"), w)
	}
	if q := a.paneQuery(); q != "" {
		cur, total := a.pane.MatchPos()
		right := th.barDim.Render("no matches ")
		if total > 0 {
			count := plural(total, "match", "matches")
			if cur > 0 {
				count = strconv.Itoa(cur) + "/" + strconv.Itoa(total)
			}
			if n := a.crossTotal(); n > 0 {
				count += " in file · " + strconv.Itoa(n) + " in all"
			}
			right = th.bar.Render(count) + th.barDim.Render("  "+a.hint(ctxNormal, "next_match")+"/"+a.hint(ctxNormal, "prev_match")+" · "+a.hint(ctxNormal, "toggle_results")+" list ")
		}
		left := th.barPrompt.Render(" / ") + th.bar.Render(q)
		gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
		return fit(left+strings.Repeat(" ", max(gap, 1))+right, w)
	}
	return fit(th.barDim.Render(" "+a.hint(ctxNormal, "search_file")+" search file   "+a.hint(ctxNormal, "search_files")+" search "+
		a.crossScope.String()+"   "+a.hint(ctxNormal, "toggle_search_bar")+" hide bar   "+a.hint(ctxNormal, "help")+" keys"), w)
}

// typingBar is what flanks the input while typing: the prompt on the left,
// and the mode and scope chips on the right.
func (a *App) typingBar() (left, right string) {
	th := a.theme
	prompt := "/"
	if a.scope != search.File {
		prompt = "?"
	}
	left = th.barPrompt.Render(" " + prompt + " ")
	var chips strings.Builder
	for s := search.File; s <= search.Repo; s++ {
		if s == a.scope {
			chips.WriteString(th.barChipOn.Render(" " + s.String() + " "))
		} else {
			chips.WriteString(th.barChip.Render(" " + s.String() + " "))
		}
	}
	mode := th.barChip.Render(" regex ")
	if a.mode.Literal {
		mode = th.barChipOn.Render(" literal ")
	}
	return left, " " + mode + " " + chips.String() + " "
}

// inputWidth is the room the search input has between the typing bar's
// prompt and chips.
func (a *App) inputWidth() int {
	left, right := a.typingBar()
	return max(a.width-ansi.StringWidth(left)-ansi.StringWidth(right)-1, 1)
}

// crossTotal is the number of matches across all files when the current
// pane's matches came from a directory or repo search, or 0.
func (a *App) crossTotal() int {
	r := a.results
	if r == nil || r.req.Scope == search.File || a.pane.match == nil ||
		r.req.Query != a.pane.match.query || r.req.Mode != a.pane.match.mode {
		return 0
	}
	return r.Len()
}

// paneQuery is the current pane's search, or "" when no file is open.
func (a *App) paneQuery() string {
	if a.pane == nil {
		return ""
	}
	return a.pane.Query()
}
