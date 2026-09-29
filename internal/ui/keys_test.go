package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/search"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestKeysMD(t *testing.T) {
	want := DefaultKeymap().Markdown()
	path := filepath.Join("..", "..", "KEYS.md")
	if os.Getenv("UPDATE_KEYS_MD") != "" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("KEYS.md is out of date: UPDATE_KEYS_MD=1 go test ./internal/ui -run TestKeysMD")
	}
}

func TestActionTable(t *testing.T) {
	seen := map[string]bool{}
	known := map[string]bool{}
	for _, g := range groups {
		known[g] = true
	}
	for _, x := range actions {
		id := string(x.ctx) + "." + x.name
		if seen[id] {
			t.Errorf("%s defined twice", id)
		}
		seen[id] = true
		if x.run == nil || x.desc == "" || !known[x.group] {
			t.Errorf("%s: missing run, description or group", id)
		}
	}
	// The defaults have no conflicts: NewKeymap would fail on a key that is
	// both bound and a sequence prefix, and no key may be bound twice.
	km := DefaultKeymap()
	for _, x := range actions {
		for _, k := range x.keys {
			if km.bind[x.ctx][k] != x {
				t.Errorf("%s: %q is also bound to %s", string(x.ctx)+"."+x.name, k, km.bind[x.ctx][k].name)
			}
		}
	}
}

func TestDumpConfigParses(t *testing.T) {
	tmpl := config.Template(Bindings())
	c := config.Default()
	if err := config.Parse([]byte(tmpl), &c); err != nil {
		t.Fatalf("template: %v", err)
	}
	// Uncommenting every key line gives back the defaults.
	var sb strings.Builder
	for _, l := range strings.Split(tmpl, "\n") {
		if strings.HasPrefix(l, "# ") && strings.Contains(l, " = [") {
			l = strings.TrimPrefix(l, "# ")
		}
		sb.WriteString(l + "\n")
	}
	c = config.Default()
	if err := config.Parse([]byte(sb.String()), &c); err != nil {
		t.Fatalf("uncommented template: %v", err)
	}
	km, err := NewKeymap(c.Keys)
	if err != nil {
		t.Fatal(err)
	}
	if km.Markdown() != DefaultKeymap().Markdown() {
		t.Fatal("uncommented keys should equal the defaults")
	}
}

func TestConfigParse(t *testing.T) {
	c := config.Default()
	err := config.Parse([]byte("width = 90\n[startup]\ntoc = false\n[search]\nscope = \"dir\"\nliteral = true\n[theme]\naccent = \"#ff8800\"\n"), &c)
	if err != nil || c.Width != 90 || c.Startup.TOC || !c.Startup.Tabs || c.Search.Scope != search.Dir || !c.Search.Literal || c.Theme.Accent != "#ff8800" {
		t.Fatalf("parse: %+v %v", c, err)
	}
	for _, bad := range []string{
		"widht = 90",                 // unknown setting
		"[search]\nscope = \"all\"",  // bad scope
		"[search]\ncase = \"upper\"", // bad case
		"[theme]\naccent = \"blue\"", // bad colour
		"width = \"wide\"",           // wrong type
	} {
		c := config.Default()
		if err := config.Parse([]byte(bad), &c); err == nil {
			t.Errorf("%q should be an error", bad)
		}
	}
	if _, err := config.Load(filepath.Join(t.TempDir(), "none.toml"), false); err != nil {
		t.Errorf("a missing optional file is fine: %v", err)
	}
	if _, err := config.Load(filepath.Join(t.TempDir(), "none.toml"), true); err == nil {
		t.Error("a missing -config file is an error")
	}
}

func TestKeymapChanges(t *testing.T) {
	km, err := NewKeymap(map[string]map[string][]string{
		"normal": {"scroll_down": {"ctrl-j"}, "help": {"H"}, "open_menu": {"j"}},
		"window": {"split_vertical": {"|"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := km.Keys("normal", "scroll_down"); len(got) != 1 || got[0] != "ctrl+j" {
		t.Errorf("ctrl-j should normalise to ctrl+j: %v", got)
	}
	if x, _ := km.lookup("j", "normal"); x == nil || x.name != "open_menu" {
		t.Error("j should move to open_menu")
	}
	if x, _ := km.lookup("down", "normal"); x != nil {
		t.Error("scroll_down's other keys should be gone")
	}

	for _, bad := range []map[string]map[string][]string{
		{"nowhere": {"x": {"a"}}},
		{"normal": {"fly": {"a"}}},
		{"normal": {"help": {"g"}}},     // g starts gg, gt, ...
		{"normal": {"help": {"5"}}},     // digits are counts
		{"search": {"submit": {"a b"}}}, // no sequences while typing
	} {
		if _, err := NewKeymap(bad); err == nil {
			t.Errorf("%v should be an error", bad)
		}
	}
}

func TestReboundKeys(t *testing.T) {
	dir := fixture(t)
	km, err := NewKeymap(map[string]map[string][]string{
		"normal": {"window": {"alt+w"}, "open_menu": {"O"}},
		"window": {"split_vertical": {"|"}},
		"menu":   {"close": {"x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := doc.Load(filepath.Join(dir, "a.md"))
	a := New([]*doc.Doc{d}, Options{Style: "notty", MaxWrap: 80, Keymap: km})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(a, "alt+w", "|")
	if len(a.tab().leaves()) != 2 {
		t.Fatalf("alt+w | should split: %s", paneNames(a))
	}
	press(a, "O")
	if a.menu == nil {
		t.Fatal("O should open the menu")
	}
	press(a, "x")
	if a.menu != nil {
		t.Fatal("x should close the menu")
	}
	if help := ansi.Strip(strings.Join(a.helpLines(), "\n")); !strings.Contains(help, "alt+w |") {
		t.Error("the help should show the new keys")
	}
}

func TestHelpOverlay(t *testing.T) {
	a := newTestApp(t, filepath.Join(fixture(t), "a.md"))
	press(a, "g", "?")
	if a.help == nil {
		t.Fatal("g? should open the help")
	}
	text := ansi.Strip(strings.Join(a.helpLines(), "\n"))
	for _, want := range []string{"Scrolling", "ctrl+w v", "split side by side", "File menu"} {
		if !strings.Contains(text, want) {
			t.Errorf("help should contain %q", want)
		}
	}
	for _, w := range []int{40, 58, 98} {
		for _, l := range a.keymap.helpText(w, a.theme) {
			if ansi.StringWidth(l) > w {
				t.Errorf("help line wider than %d: %q", w, ansi.Strip(l))
			}
		}
	}
	screen := ansi.Strip(a.View().Content)
	if !strings.Contains(screen, "Keys") || !strings.Contains(screen, "HELP") {
		t.Fatalf("help should be drawn:\n%s", screen)
	}
	press(a, "G")
	if a.help.offset == 0 {
		t.Error("G should scroll to the bottom")
	}
	press(a, "j") // keys go to the help, not the document
	if a.pane.offset != 0 {
		t.Error("the document should not scroll under the help")
	}
	press(a, "esc")
	if a.help != nil {
		t.Fatal("esc should close the help")
	}
}

func TestLiteralToggle(t *testing.T) {
	a := newTestApp(t, filepath.Join(fixture(t), "a.md"))
	press(a, "/")
	if a.mode.Literal || !strings.Contains(ansi.Strip(a.barView()), "regex") {
		t.Fatal("regex by default")
	}
	a.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if !a.mode.Literal || !strings.Contains(ansi.Strip(a.barView()), "literal") {
		t.Fatal("ctrl+r should switch to literal")
	}
	typeText(a, "a.md")
	press(a, "enter")
	if !a.mode.Literal {
		t.Fatal("literal should stay on for the next search")
	}
}

func TestWindowCountAfterPrefix(t *testing.T) {
	a := newTestApp(t, filepath.Join(fixture(t), "a.md"))
	ctrlW(a, "v")
	l := a.tab().focus
	w := l.w
	a.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	press(a, "5")
	if !strings.Contains(ansi.Strip(a.statusLine()), "5ctrl+w") {
		t.Errorf("status should show the pending count: %q", ansi.Strip(a.statusLine()))
	}
	press(a, "<")
	if l.w != w-5 {
		t.Fatalf("ctrl+w 5 <: width %d, want %d", l.w, w-5)
	}
	if a.window || a.count != 0 {
		t.Fatal("the window command should be done")
	}
}

func TestPercentPastEnd(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := range 100 {
		sb.WriteString("para " + string(rune('a'+i%26)) + "\n\n")
	}
	sb.WriteString("# End\n\nlast\n")
	path := filepath.Join(dir, "p.md")
	os.WriteFile(path, []byte(sb.String()), 0o644)
	a := newTestApp(t, path)
	press(a, "]", "]")
	p := a.pane
	if p.offset <= p.maxOffset() {
		t.Skip("the heading jump didn't go past the end")
	}
	if got := p.Percent(); got == "Bot" || !strings.HasSuffix(got, "%") {
		t.Fatalf("past the end: %q", got)
	}
	p.ScrollToBottom()
	if p.Percent() != "Bot" {
		t.Fatal("at the end: Bot")
	}
}

func TestScrollBindSourceLines(t *testing.T) {
	dir := t.TempDir()
	var short, long strings.Builder
	for i := range 200 {
		short.WriteString("line\n\n")
		// Long lines wrap, so rendered lines and source lines differ.
		long.WriteString(strings.Repeat("word ", 40) + "\n\n")
		_ = i
	}
	sp, lp := filepath.Join(dir, "short.md"), filepath.Join(dir, "long.md")
	os.WriteFile(sp, []byte(short.String()), 0o644)
	os.WriteFile(lp, []byte(long.String()), 0o644)
	a := newTestApp(t, sp)
	ctrlW(a, "v")
	a.openAt(lp, openReplace)
	ctrlW(a, "b")
	ctrlW(a, "h")
	ctrlW(a, "b")
	l, r := a.tab().leaves()[0].pane, a.tab().leaves()[1].pane
	press(a, "1", "0", "j")
	if l.TopSource() != r.TopSource() {
		t.Fatalf("bound panes should share the source line: %d %d", l.TopSource(), r.TopSource())
	}
	if l.offset == r.offset {
		t.Fatal("the long file's rendered offset should differ")
	}
}

func TestMouseClearsMessage(t *testing.T) {
	a := newTestApp(t, filepath.Join(fixture(t), "a.md"))
	a.msg = "stale"
	a.Update(tea.MouseWheelMsg{X: 50, Y: 5, Button: tea.MouseWheelDown})
	if a.msg != "" {
		t.Fatal("the wheel should clear the message")
	}
	a.msg = "stale"
	a.Update(tea.MouseClickMsg{X: 50, Y: 5, Button: tea.MouseLeft})
	if a.msg != "" {
		t.Fatal("a click should clear the message")
	}
}

func TestEscKillsSearch(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "a.md")
	a := newTestApp(t, path)
	p, req := a.pane, search.Request{Query: "text", Scope: search.File, Root: path}
	res := search.Result{Matches: []search.Match{{Path: path, Line: 3, Text: "text", Spans: [][2]int{{0, 4}}}}}
	a.seq++
	a.update(searchDoneMsg{seq: a.seq, pane: p, req: req, res: res, pre: indexed(req, res.Matches, p.doc, p.view)})
	if a.pane.match == nil {
		t.Fatal("expected highlights after the search")
	}
	press(a, "esc") // an executed search: nothing stays highlighted
	if a.pane.match != nil {
		t.Fatal("esc should clear every highlight")
	}
	press(a, "n")
	if a.msg != "no previous search" {
		t.Fatalf("a killed search should be forgotten, msg=%q", a.msg)
	}
}

func TestEscDropsResults(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "a.md")
	search := func(a *App) {
		t.Helper()
		p, req := a.pane, search.Request{Query: "text", Scope: search.Dir, Root: dir}
		res := search.Result{Files: 1, Matches: []search.Match{{Path: path, Line: 3, Text: "text", Spans: [][2]int{{0, 4}}}}}
		a.seq++
		a.update(searchDoneMsg{seq: a.seq, pane: p, req: req, res: res, pre: indexed(req, res.Matches, p.doc, p.view)})
		if !a.showResults || a.focus != focusResults {
			t.Fatal("a dir search should open the results list")
		}
	}
	for name, keys := range map[string][]string{
		"esc in the bar":    {"/", "x", "esc"},
		"ctrl+c in the bar": {"/", "x", "ctrl+c", "ctrl+c"},
		"esc in the pane":   {"esc"},
		"esc in the list":   {"esc"},
		"q in the list":     {"q"},
	} {
		a := newTestApp(t, path)
		search(a)
		if name == "esc in the pane" {
			a.focus = focusDoc
		}
		press(a, keys...)
		if a.typing || a.showResults || a.results != nil || a.focus == focusResults || a.pane.match != nil {
			t.Fatalf("%s: the results list and highlights should be gone", name)
		}
		press(a, "g", "r")
		if a.showResults || a.msg != "no search results" {
			t.Fatalf("%s: gr should find no results, msg=%q", name, a.msg)
		}
	}
}

func TestEscInBarKillsRunningSearch(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"))
	press(a, "/")
	typeText(a, "text")
	cmd := press(a, "enter") // the search runs in the background
	if cmd == nil || a.searching == "" {
		t.Fatal("submitting should start a background search")
	}
	press(a, "/")
	if !a.typing {
		t.Fatal("should be typing")
	}
	press(a, "esc")
	if a.typing || a.searching != "" || a.cancel != nil {
		t.Fatal("esc in the bar should stop typing and kill the running search")
	}
	a.update(cmd().(searchDoneMsg)) // the killed search lands late: ignored
	if a.pane.match != nil {
		t.Fatal("a killed search must never highlight")
	}
}

func TestCtrlGShowsFullPath(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"))
	if !filepath.IsAbs(a.pane.doc.Path) {
		t.Fatalf("doc.Path should be absolute: %q", a.pane.doc.Path)
	}
	a.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if a.msg != a.pane.doc.Path || !filepath.IsAbs(a.msg) {
		t.Fatalf("ctrl+g should list the full path, got %q", a.msg)
	}
}

func TestBackgroundIndex(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for range 50 {
		sb.WriteString("a needle here\n\n")
	}
	path := filepath.Join(dir, "n.md")
	os.WriteFile(path, []byte(sb.String()), 0o644)
	a := newTestApp(t, path)
	p := a.pane
	req := search.Request{Query: "needle", Scope: search.File, Root: path}
	res := search.Result{}
	for i := range 50 {
		res.Matches = append(res.Matches, search.Match{Path: path, Line: 2*i + 1, Text: "a needle here", Spans: [][2]int{{2, 8}}})
	}
	pre := indexed(req, res.Matches, p.doc, p.view)
	a.update(searchDoneMsg{seq: a.seq, pane: p, req: req, res: res, pre: pre})
	if p.match != pre {
		t.Fatal("an index built for the current rendering should be used as is")
	}
	if _, total := p.MatchPos(); total != 50 {
		t.Fatalf("matches: %d", total)
	}
	// Built for an older rendering, it gets re-indexed.
	stale := indexed(req, res.Matches, p.doc, nil)
	p.match = nil
	p.setMatches(stale)
	if stale.view != p.view || len(stale.target) != 50 {
		t.Fatal("a stale index should be rebuilt")
	}
}

func TestReadmeConfigExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(b), "```toml\n")
	example, _, ok2 := strings.Cut(rest, "```")
	if !ok || !ok2 {
		t.Fatal("README should have a toml example")
	}
	c := config.Default()
	if err := config.Parse([]byte(example), &c); err != nil {
		t.Fatalf("README example: %v", err)
	}
	if _, err := NewKeymap(c.Keys); err != nil {
		t.Fatalf("README example keys: %v", err)
	}
}

func TestStatusKeepsPositionWithLongName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, strings.Repeat("very-long-name-", 10)+".md")
	if err := os.WriteFile(p, []byte("# t\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, p)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	s := ansi.Strip(a.statusLine())
	if ansi.StringWidth(s) != 60 {
		t.Errorf("status should be exactly the width, got %d: %q", ansi.StringWidth(s), s)
	}
	if !strings.HasSuffix(s, "Ln 1/3  All ") || !strings.Contains(s, "…") {
		t.Errorf("status should keep the position and elide the name: %q", s)
	}
}
