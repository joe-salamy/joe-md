package ui

import (
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"
)

// Closing a tab or pane pushes it on a stack, and X reopens it where it was,
// like a browser's reopen-closed-tab. The stack keeps the tabs and nodes
// themselves, so a reopened pane is exactly as it was left, and a pane
// reopened beside a neighbour that was itself closed and reopened finds it.

const closedMax = 20

type closedKind int

const (
	closedTab    closedKind = iota // x, q on a tab's last pane, middle click
	closedPane                     // q in a tab with several panes
	closedOthers                   // ctrl+w o
)

type closed struct {
	kind closedKind
	tab  *Tab
	idx  int // closedTab: its place in the tab bar

	// closedPane: the leaf, and where it was: beside (vert) or above or
	// below the node that took its space, before or after it, with its
	// share of their parent. heirKids are the heir's children, in case
	// closing merged it away.
	node         *Node
	heir         *Node
	heirKids     []*Node
	vert, before bool
	frac         float64

	// closedOthers: the tree ctrl+w o cut the kept pane out of, and the
	// kept pane's place in it.
	root, kept *Node
	keptParent *Node
	keptIdx    int
	keptFrac   float64
}

func (a *App) pushClosed(e *closed) {
	a.closed = append(a.closed, e)
	if len(a.closed) > closedMax {
		a.closed = slices.Delete(a.closed, 0, len(a.closed)-closedMax)
	}
}

// closedPaneOf records where the focused pane of t is, before t.close.
func closedPaneOf(t *Tab) *closed {
	f := t.focus
	par := f.parent
	i := f.index()
	e := &closed{kind: closedPane, tab: t, node: f, vert: par.vert, frac: f.frac}
	if i == 0 {
		e.heir, e.before = par.kids[1], true
	} else {
		e.heir = par.kids[i-1]
	}
	e.heirKids = slices.Clone(e.heir.kids)
	return e
}

// closeOthers is ctrl+w o: close every pane of the tab but the focused one.
func (a *App) closeOthers() {
	t := a.tab()
	f := t.focus
	if f.parent == nil {
		return
	}
	a.pushClosed(&closed{kind: closedOthers, tab: t, root: t.root, kept: f,
		keptParent: f.parent, keptIdx: f.index(), keptFrac: f.frac})
	t.only()
	a.layout()
}

// tabIndex is the index of t in the tab bar, or -1 if it is closed.
func (a *App) tabIndex(t *Tab) int { return slices.Index(a.tabs, t) }

// reopen is X: reopen the last n closed tabs or panes, newest first. Panes
// whose file is gone are skipped. The command re-runs the searches of panes
// whose file changed while they were closed.
func (a *App) reopen(n int) tea.Cmd {
	if len(a.closed) == 0 {
		a.msg = "nothing to reopen"
		return nil
	}
	var cmds []tea.Cmd
	for ; n > 0 && len(a.closed) > 0; n-- {
		e := a.closed[len(a.closed)-1]
		a.closed = a.closed[:len(a.closed)-1]
		cmd, ok := a.reopenOne(e)
		if !ok {
			n++ // a skipped pane doesn't count
		}
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (a *App) reopenOne(e *closed) (tea.Cmd, bool) {
	var reload []*Node
	switch e.kind {
	case closedTab:
		i := min(e.idx, len(a.tabs))
		a.tabs = slices.Insert(a.tabs, i, e.tab)
		a.activate(i)
		reload = e.tab.leaves()
	case closedPane:
		if _, err := os.Stat(e.node.pane.doc.Path); err != nil {
			a.msg = "reopen: " + err.Error()
			return nil, false
		}
		a.reopenPane(e)
		reload = []*Node{e.node}
	case closedOthers:
		a.reopenOthers(e)
		reload = a.tab().leaves()
	}
	var cmds []tea.Cmd
	for _, l := range reload {
		changed, err := l.pane.Reload(a.renderer, a.opts.MaxWrap)
		if err != nil {
			a.msg = "reopen: " + err.Error()
		}
		if changed {
			cmds = append(cmds, a.refreshSearch(l.pane))
		}
	}
	a.layout()
	return tea.Batch(cmds...), true
}

// reopenPane puts a closed pane back beside the node that took its space,
// or, if that is gone, beside its tab's focused pane. If the tab is closed
// too, the pane gets a new tab.
func (a *App) reopenPane(e *closed) {
	n := e.node
	n.parent, n.frac = nil, 1
	ti := a.tabIndex(e.tab)
	if ti < 0 {
		i := min(a.cur+1, len(a.tabs))
		a.tabs = slices.Insert(a.tabs, i, &Tab{root: n, focus: n})
		a.activate(i)
		return
	}
	t := e.tab
	m := e.heir
	if !t.has(m) {
		m = t.regroup(m, e.heirKids)
	}
	share := e.frac
	if m == nil {
		m, share = t.focus, 0.5
	} else if m.parent != nil && m.parent.vert == e.vert {
		share = e.frac / m.frac // m has the pane's space as well as its own
	}
	t.insert(n, m, e.vert, e.before, share)
	a.cur = ti
	a.focusLeaf(n)
}

// regroup rebuilds inner node h from its children when closing a pane next
// to it merged it into a parent laid out the same way. It returns the node
// now standing where h stood, or nil if the children have moved since.
func (t *Tab) regroup(h *Node, kids []*Node) *Node {
	if h.pane != nil || len(kids) == 0 {
		return nil
	}
	p := kids[0].parent
	if p == nil || p.vert != h.vert || !t.has(p) {
		return nil
	}
	i := kids[0].index()
	if i < 0 || i+len(kids) > len(p.kids) || !slices.Equal(p.kids[i:i+len(kids)], kids) {
		return nil
	}
	if len(kids) == len(p.kids) {
		return p
	}
	sum := 0.0
	for _, k := range kids {
		sum += k.frac
	}
	for _, k := range kids {
		k.parent, k.frac = h, k.frac/sum
	}
	h.kids, h.frac, h.parent = slices.Clone(kids), sum, p
	p.kids = slices.Concat(p.kids[:i], []*Node{h}, p.kids[i+len(kids):])
	return h
}

// reopenOthers undoes ctrl+w o: the tree comes back with whatever the tab
// holds now in the kept pane's place. If the tab is closed, the tree gets a
// new tab with a copy of the kept pane.
func (a *App) reopenOthers(e *closed) {
	cur := leaf(e.kept.pane.clone())
	ti := a.tabIndex(e.tab)
	t := e.tab
	if ti >= 0 {
		cur = t.root
	} else {
		t = &Tab{focus: cur}
		ti = min(a.cur+1, len(a.tabs))
		a.tabs = slices.Insert(a.tabs, ti, t)
	}
	cur.parent, cur.frac = e.keptParent, e.keptFrac
	e.keptParent.kids[e.keptIdx] = cur
	t.root = e.root
	cur.merge()
	a.cur = ti
	a.focusLeaf(t.focus)
}
