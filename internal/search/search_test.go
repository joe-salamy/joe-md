package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMatch(t *testing.T) {
	line := `{"type":"match","data":{"path":{"text":"/a/b.md"},"lines":{"text":"# Hi hello\r\n"},"line_number":7,"absolute_offset":0,"submatches":[{"match":{"text":"Hello"},"start":5,"end":10}]}}`
	m, ok := parseMatch([]byte(line))
	if !ok {
		t.Fatal("not parsed")
	}
	if m.Path != "/a/b.md" || m.Line != 7 || m.Text != "# Hi hello" {
		t.Fatalf("got %+v", m)
	}
	if len(m.Spans) != 1 || m.Text[m.Spans[0][0]:m.Spans[0][1]] != "hello" {
		t.Fatalf("spans %v", m.Spans)
	}
	if _, ok := parseMatch([]byte(`{"type":"begin","data":{"path":{"text":"x"}}}`)); ok {
		t.Fatal("begin parsed as match")
	}
	// Non-UTF-8 data arrives base64 encoded.
	m, ok = parseMatch([]byte(`{"type":"match","data":{"path":{"bytes":"eC5tZA=="},"lines":{"text":"x"},"line_number":1,"submatches":[]}}`))
	if !ok || m.Path != "x.md" {
		t.Fatalf("bytes path: %+v", m)
	}
}

func TestScope(t *testing.T) {
	if File.Next(1) != Dir || Repo.Next(1) != File || File.Next(-1) != Repo {
		t.Fatal("Next does not cycle")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(sub, "x.md")
	if got := Root(f, File); got != f {
		t.Errorf("File root %q", got)
	}
	if got := Root(f, Dir); got != sub {
		t.Errorf("Dir root %q", got)
	}
	if got := Root(f, Repo); got != dir {
		t.Errorf("Repo root %q", got)
	}
}

func TestRun(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a.md", "one\nSetup here\nthree setup\n")
	write("b.md", "nothing\n")
	write("c.go", "setup in go\n")

	ctx := context.Background()
	res, err := Run(ctx, Request{Query: "setup", Scope: Dir, Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 2 || res.Files != 1 || res.Matches[0].Line != 2 || res.Matches[1].Line != 3 {
		t.Fatalf("dir search (markdown only, case-insensitive): %+v", res)
	}

	res, err = Run(ctx, Request{Query: "setup", Scope: Dir, Root: dir, Limit: 1})
	if err != nil || len(res.Matches) != 1 || !res.Truncated {
		t.Fatalf("limit: %+v %v", res, err)
	}

	res, err = Run(ctx, Request{Query: "absent", Scope: File, Root: a})
	if err != nil || len(res.Matches) != 0 {
		t.Fatalf("no match should not be an error: %+v %v", res, err)
	}

	if _, err = Run(ctx, Request{Query: "(", Scope: File, Root: a}); err == nil {
		t.Fatal("bad regex should be an error")
	}

	write("d.md", "a.b\naxb\n(x)\n")
	d := filepath.Join(dir, "d.md")
	res, err = Run(ctx, Request{Query: "a.b", Mode: Mode{Literal: true}, Scope: File, Root: d})
	if err != nil || len(res.Matches) != 1 || res.Matches[0].Line != 1 {
		t.Fatalf("literal: %+v %v", res, err)
	}
	res, err = Run(ctx, Request{Query: "(", Mode: Mode{Literal: true}, Scope: File, Root: d})
	if err != nil || len(res.Matches) != 1 {
		t.Fatalf("literal (: %+v %v", res, err)
	}

	for _, tc := range []struct {
		q    string
		c    Case
		want int
	}{{"setup", IgnoreCase, 2}, {"setup", SmartCase, 2}, {"Setup", SmartCase, 1}, {"SETUP", SmartCase, 0}, {"setup", MatchCase, 1}} {
		res, err = Run(ctx, Request{Query: tc.q, Mode: Mode{Case: tc.c}, Scope: File, Root: a})
		if err != nil || len(res.Matches) != tc.want {
			t.Errorf("%q with case %v: %d matches, want %d (%v)", tc.q, tc.c, len(res.Matches), tc.want, err)
		}
	}
}

func TestTerms(t *testing.T) {
	for _, tc := range []struct {
		q       string
		literal bool
		want    []string
	}{
		{"", false, nil},
		{"  alpha\tneedle\nomega  ", false, []string{"alpha", "needle", "omega"}},
		{"   ", true, nil},
		// A literal query is one phrase, trimmed.
		{"  shut up ", true, []string{"shut up"}},
	} {
		got := Request{Query: tc.q, Mode: Mode{Literal: tc.literal}}.Terms()
		if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
			t.Errorf("Terms(%q, literal=%v) = %q, want %q", tc.q, tc.literal, got, tc.want)
		}
	}
}

func TestPatterns(t *testing.T) {
	for _, tc := range []struct {
		q    string
		mode Mode
		text string
		want []string // what each pattern finds in text, joined by ","
	}{
		{`\bup\b`, Mode{}, "setup UP up", []string{"UP,up"}},
		{"up", Mode{Case: MatchCase}, "setup UP up", []string{"up,up"}},
		{"Up", Mode{Case: SmartCase}, "Up up", []string{"Up"}},
		{"up", Mode{Case: SmartCase}, "Up up", []string{"Up,up"}},
		// An escape is not a capital for smart case.
		{`up\W`, Mode{Case: SmartCase}, "UP! up!", []string{"UP!,up!"}},
		{"a.b c", Mode{Literal: true}, "a.b c, axb c, A.B C", []string{"a.b c,A.B C"}},
		{"alpha needle", Mode{}, "Alpha needle", []string{"Alpha", "needle"}},
		// A phrase skips whitespace and inline markup between its words.
		{"one fixed string", Mode{Literal: true}, "one **fixed** string, one `fixed`\n  string, one [fixed](a.md) string",
			[]string{"one **fixed** string,one `fixed`\n  string,one [fixed](a.md) string"}},
		{"one fixed", Mode{Literal: true}, "one\u00a0fixed, onefixed, one - fixed", []string{"one\u00a0fixed"}},
	} {
		pats, err := Request{Query: tc.q, Mode: tc.mode}.Patterns()
		if err != nil || len(pats) != len(tc.want) {
			t.Fatalf("%q: %v %v", tc.q, pats, err)
		}
		for i, re := range pats {
			if got := strings.Join(re.FindAllString(tc.text, -1), ","); got != tc.want[i] {
				t.Errorf("%q (%+v) in %q: %q, want %q", tc.q, tc.mode, tc.text, got, tc.want[i])
			}
		}
	}
	if _, err := (Request{Query: "("}).Patterns(); err == nil {
		t.Error("a bad regex should be an error")
	}
}

func TestRunMultiTerm(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a.md", "alpha needle alpha\nonly alpha here\nonly needle here\nneither\n")
	ctx := context.Background()

	res, err := Run(ctx, Request{Query: "alpha needle", Scope: File, Root: a})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 || res.Matches[0].Line != 1 {
		t.Fatalf("AND on one line: %+v", res)
	}
	m := res.Matches[0]
	if len(m.Spans) != 3 {
		t.Fatalf("merged spans %v in %q", m.Spans, m.Text)
	}
	if terms := m.Terms(); len(terms) != 2 || terms[0] != "alpha" || terms[1] != "needle" {
		t.Fatalf("terms %q", terms)
	}

	// No single line holds all three: AND, not OR.
	res, err = Run(ctx, Request{Query: "alpha needle here", Scope: File, Root: a})
	if err != nil || len(res.Matches) != 0 {
		t.Fatalf("three terms: %+v %v", res, err)
	}

	// Sharing a file is not enough; the terms must share a line.
	b := write("b.md", "alpha\nneedle\n")
	res, err = Run(ctx, Request{Query: "alpha needle", Scope: File, Root: b})
	if err != nil || len(res.Matches) != 0 {
		t.Fatalf("different lines: %+v %v", res, err)
	}

	// Across files, only the co-occurring line comes back.
	res, err = Run(ctx, Request{Query: "alpha needle", Scope: Dir, Root: dir})
	if err != nil || len(res.Matches) != 1 || res.Files != 1 || res.Matches[0].Path != a {
		t.Fatalf("dir scope: %+v %v", res, err)
	}

	// A literal query is one phrase, not terms to AND.
	d := write("d.md", "a.b plus c\naxb plus c\nsay a.b c\n")
	res, err = Run(ctx, Request{Query: " a.b c ", Mode: Mode{Literal: true}, Scope: File, Root: d})
	if err != nil || len(res.Matches) != 1 || res.Matches[0].Line != 3 {
		t.Fatalf("literal phrase: %+v %v", res, err)
	}

	// ripgrep skips markup between the words of a phrase too.
	e := write("e.md", "one **fixed** string\none _fixed_  string\none `fixed` string\none fixed\nstring\n")
	res, err = Run(ctx, Request{Query: "one fixed string", Mode: Mode{Literal: true}, Scope: File, Root: e})
	if err != nil || len(res.Matches) != 3 || res.Matches[2].Line != 3 {
		t.Fatalf("phrase with markup: %+v %v", res, err)
	}
	if got := res.Matches[0].Terms(); len(got) != 1 || got[0] != "one **fixed** string" {
		t.Fatalf("phrase span: %q", got)
	}

	// A repeated term runs once: no duplicated spans.
	res, err = Run(ctx, Request{Query: "alpha alpha", Scope: File, Root: a})
	if err != nil || len(res.Matches) != 2 || len(res.Matches[0].Spans) != 2 {
		t.Fatalf("repeated term: %+v %v", res, err)
	}

	// A bad regex in any term is still an error.
	if _, err = Run(ctx, Request{Query: "( alpha", Scope: File, Root: a}); err == nil {
		t.Fatal("bad regex in one term should be an error")
	}
}
