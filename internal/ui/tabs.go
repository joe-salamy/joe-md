package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/doc"

	"github.com/charmbracelet/x/ansi"
)

const tabMaxName = 28

// openMode says where openAt puts a file.
type openMode int

const (
	openNewTab  openMode = iota // a new tab after the current one
	openReplace                 // the focused pane, replacing its file
	openVSplit                  // a new pane right of the focused one
	openHSplit                  // a new pane below the focused one
)

// activate makes tab i the one on screen.
func (a *App) activate(i int) {
	if len(a.tabs) == 0 {
		a.cur, a.pane, a.toc = 0, nil, nil
		return
	}
	a.cur = max(0, min(i, len(a.tabs)-1))
	a.focusLeaf(a.tabs[a.cur].focus)
}

// focusLeaf gives pane n of the current tab the focus. With activate, it is
// the only place that sets a.pane and a.toc.
func (a *App) focusLeaf(n *Node) {
	t := a.tabs[a.cur]
	t.focus = n
	a.pane, a.toc = n.pane, &n.pane.toc
	a.layout() // hidden tabs are laid out lazily; unchanged sizes cost nothing
}

func (a *App) tab() *Tab {
	if len(a.tabs) == 0 {
		return nil
	}
	return a.tabs[a.cur]
}

// tabOf returns the index of a tab showing path in any pane, or -1. The
// current tab is checked first.
func (a *App) tabOf(path string) int {
	if t := a.tab(); t != nil && t.leafOf(path) != nil {
		return a.cur
	}
	for i, t := range a.tabs {
		if t.leafOf(path) != nil {
			return i
		}
	}
	return -1
}

// switchTo shows the pane displaying path, in whichever tab it is.
func (a *App) switchTo(i int, path string) {
	a.cur = i
	a.focusLeaf(a.tabs[i].leafOf(path))
}

// openAt makes path the document on screen. Opening in a tab or the focused
// pane switches to a pane already showing the file, if any; splits always
// open it again, to show two places in one file. Everything that opens
// files goes through here. Open files are known by absolute path (see
// doc.Load), so paths compare with ==.
func (a *App) openAt(path string, mode openMode) bool {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if mode >= openVSplit && a.pane == nil {
		mode = openNewTab
	}
	if mode < openVSplit {
		if i := a.tabOf(path); i >= 0 {
			a.switchTo(i, path)
			return true
		}
	}
	if mode >= openVSplit && !a.tab().canSplit(mode == openVSplit) {
		a.msg = "no room to split"
		return false
	}
	d, err := doc.Load(path)
	if err != nil {
		a.msg = "open: " + err.Error()
		return false
	}
	p := NewPane(d)
	switch {
	case mode == openReplace && a.pane != nil:
		t := a.tab()
		n := leaf(p)
		t.replace(t.focus, n)
		a.focusLeaf(n)
	case mode >= openVSplit:
		a.tab().split(p, mode == openVSplit)
		a.focusLeaf(a.tab().focus)
	default:
		i := min(a.cur+1, len(a.tabs))
		a.tabs = slices.Insert(a.tabs, i, newTab(p))
		a.activate(i)
	}
	return true
}

// splitPane is ctrl+w v / ctrl+w s: the focused pane again, beside or below.
func (a *App) splitPane(vert bool) {
	t := a.tab()
	if !t.canSplit(vert) {
		a.msg = "no room to split"
		return
	}
	t.split(a.pane.clone(), vert)
	a.focusLeaf(t.focus)
}

// closePane closes the focused pane, or its tab when it is the only one, and
// reports whether anything is left open.
func (a *App) closePane() bool {
	t := a.tab()
	if t == nil {
		return false
	}
	if !t.multi() {
		return a.closeTab()
	}
	a.pushClosed(closedPaneOf(t))
	t.close()
	a.focusLeaf(t.focus)
	return true
}

// allPanes is every pane in every tab.
func (a *App) allPanes() []*Pane {
	var out []*Pane
	for _, t := range a.tabs {
		for _, l := range t.leaves() {
			out = append(out, l.pane)
		}
	}
	return out
}

// livePane reports whether p is still open somewhere.
func (a *App) livePane(p *Pane) bool {
	for _, q := range a.allPanes() {
		if q == p {
			return true
		}
	}
	return false
}

// closeTab closes the current tab and reports whether any are left. Like
// vim, the tab to the right takes its place.
func (a *App) closeTab() bool {
	if len(a.tabs) == 0 {
		return false
	}
	a.pushClosed(&closed{kind: closedTab, tab: a.tabs[a.cur], idx: a.cur})
	a.tabs = slices.Delete(a.tabs, a.cur, a.cur+1)
	if len(a.tabs) == 0 {
		a.activate(0)
		return false
	}
	a.activate(min(a.cur, len(a.tabs)-1))
	return true
}

// gotoTab is {n}gt and alt+n: tab n, 1-based.
func (a *App) gotoTab(n int) {
	if n < 1 || n > len(a.tabs) {
		a.msg = "no tab " + strconv.Itoa(n)
		return
	}
	a.activate(n - 1)
}

// stepTab is gt / gT: n tabs to the right (left if n < 0), wrapping.
func (a *App) stepTab(n int) {
	if len(a.tabs) == 0 {
		return
	}
	k := len(a.tabs)
	a.activate(((a.cur+n)%k + k) % k)
}

// moveTab is << / >>: the current tab n places to the right (left if
// n < 0), stopping at the ends.
func (a *App) moveTab(n int) {
	if len(a.tabs) == 0 {
		return
	}
	to := max(0, min(a.cur+n, len(a.tabs)-1))
	if to == a.cur {
		a.msg = "tab is already at the end"
		return
	}
	t := a.tabs[a.cur]
	a.tabs = slices.Insert(slices.Delete(a.tabs, a.cur, a.cur+1), to, t)
	a.cur = to
}

func (a *App) tabBarHeight() int {
	if a.showTabs && len(a.tabs) > 0 {
		return 1
	}
	return 0
}

// tabLabels are the tab titles: the focused pane's file name, with the parent directory
// added when two open files share a name.
func (a *App) tabLabels() []string {
	count := map[string]int{}
	for _, t := range a.tabs {
		count[t.focus.pane.doc.Name]++
	}
	out := make([]string, len(a.tabs))
	for i, t := range a.tabs {
		d := t.focus.pane.doc
		name := d.Name
		if count[name] > 1 {
			name = filepath.Join(filepath.Base(filepath.Dir(d.Path)), name)
		}
		out[i] = " " + strconv.Itoa(i+1) + " " + ansi.Truncate(name, tabMaxName, "…")
		if n := len(t.leaves()); n > 1 {
			out[i] += " +" + strconv.Itoa(n-1) // the tab's other panes
		}
		out[i] += " "
	}
	return out
}

// tabSpans lays the tab bar out in width cells: the tabs shown, first..last,
// and the column each one starts at. When they do not all fit, the window
// slides so the current tab is always visible, with arrows marking the
// hidden ones.
func (a *App) tabSpans(width int) (labels []string, first, last int, xs []int) {
	labels = a.tabLabels()
	w := make([]int, len(labels))
	for i, l := range labels {
		w[i] = ansi.StringWidth(l) + 1 // +1 for the gap after each tab
	}
	const arrow = 2
	fits := func(first, last int) bool {
		sum := 0
		for i := first; i <= last; i++ {
			sum += w[i]
		}
		if first > 0 {
			sum += arrow
		}
		if last < len(w)-1 {
			sum += arrow
		}
		return sum <= width
	}
	first = 0
	for first < a.cur && !fits(first, a.cur) {
		first++
	}
	last = a.cur
	for last+1 < len(w) && fits(first, last+1) {
		last++
	}
	x := 0
	if first > 0 {
		x = arrow
	}
	for i := first; i <= last; i++ {
		xs = append(xs, x)
		x += w[i]
	}
	return labels, first, last, xs
}

// tabBar is the one-line tab bar at the top of the screen.
func (a *App) tabBar() string {
	th := a.theme
	labels, first, last, _ := a.tabSpans(a.width)
	var sb strings.Builder
	if first > 0 {
		sb.WriteString(th.tabFill.Render("‹ "))
	}
	for i := first; i <= last; i++ {
		if i == a.cur {
			sb.WriteString(th.tabActive.Render(labels[i]))
		} else {
			sb.WriteString(th.tabInactive.Render(labels[i]))
		}
		sb.WriteString(th.tabFill.Render(" "))
	}
	line := sb.String()
	right := ""
	if last < len(labels)-1 {
		right = th.tabFill.Render(" ›")
	}
	gap := a.width - ansi.StringWidth(line) - ansi.StringWidth(right)
	return fit(line+th.tabFill.Render(strings.Repeat(" ", max(gap, 0)))+right, a.width)
}

// tabAt returns the tab under column x of the tab bar, or -1.
func (a *App) tabAt(x int) int {
	labels, first, _, xs := a.tabSpans(a.width)
	for j, start := range xs {
		i := first + j
		if x >= start && x < start+ansi.StringWidth(labels[i]) {
			return i
		}
	}
	return -1
}
