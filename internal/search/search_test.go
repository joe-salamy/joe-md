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
	if File.Next(1) != Open || Open.Next(1) != Dir || Dir.Next(1) != Repo || Repo.Next(1) != File || File.Next(-1) != Repo {
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

func TestPattern(t *testing.T) {
	for _, tc := range []struct {
		q    string
		mode Mode
		text string
		want string // what the pattern finds in text, joined by ","
	}{
		{`\bup\b`, Mode{}, "setup UP up", "UP,up"},
		{"up", Mode{Case: MatchCase}, "setup UP up", "up,up"},
		{"Up", Mode{Case: SmartCase}, "Up up", "Up"},
		{"up", Mode{Case: SmartCase}, "Up up", "Up,up"},
		// An escape is not a capital for smart case.
		{`up\W`, Mode{Case: SmartCase}, "UP! up!", "UP!,up!"},
		// A regex is one pattern, spaces and all: words must be side by side.
		{" people v. ", Mode{}, "people of the state v. smith, people vs. jones", "people vs"},
		{"a.b c", Mode{Literal: true}, "a.b c, axb c, A.B C", "a.b c,A.B C"},
		// A phrase skips whitespace and inline markup between its words.
		{"one fixed string", Mode{Literal: true}, "one **fixed** string, one `fixed`\n  string, one [fixed](a.md) string",
			"one **fixed** string,one `fixed`\n  string,one [fixed](a.md) string"},
		{"one fixed", Mode{Literal: true}, "one\u00a0fixed, onefixed, one - fixed", "one\u00a0fixed"},
		// Straight and curly quotes match each other, as do the dash forms.
		{`don't "1990-95"`, Mode{Literal: true}, "don’t “1990–95”, dont 1990 95", "don’t “1990–95”"},
		{"don’t", Mode{Literal: true}, "don't", "don't"},
	} {
		re, err := Request{Query: tc.q, Mode: tc.mode}.Pattern()
		if err != nil || re == nil {
			t.Fatalf("%q: %v %v", tc.q, re, err)
		}
		if got := strings.Join(re.FindAllString(tc.text, -1), ","); got != tc.want {
			t.Errorf("%q (%+v) in %q: %q, want %q", tc.q, tc.mode, tc.text, got, tc.want)
		}
	}
	if re, err := (Request{Query: "   "}).Pattern(); re != nil || err != nil {
		t.Errorf("blank query: %v %v", re, err)
	}
	if _, err := (Request{Query: "("}).Pattern(); err == nil {
		t.Error("a bad regex should be an error")
	}
}

func TestRunPhrases(t *testing.T) {
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
	ctx := context.Background()

	// A regex with a space is one pattern, not terms to AND.
	a := write("a.md", "alpha needle alpha needle\nneedle then alpha\nalpha\n")
	res, err := Run(ctx, Request{Query: "alpha needle", Scope: File, Root: a})
	if err != nil || len(res.Matches) != 1 || res.Matches[0].Line != 1 || len(res.Matches[0].Spans) != 2 {
		t.Fatalf("regex with a space: %+v %v", res, err)
	}

	// A literal query is one phrase.
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

	// ripgrep folds quotes and dashes the same way.
	q := write("q.md", "don’t stop\ndon't stop\n1990—95\ndont stop\n")
	res, err = Run(ctx, Request{Query: "don't stop", Mode: Mode{Literal: true}, Scope: File, Root: q})
	if err != nil || len(res.Matches) != 2 {
		t.Fatalf("quotes: %+v %v", res, err)
	}
	res, err = Run(ctx, Request{Query: "1990-95", Mode: Mode{Literal: true}, Scope: File, Root: q})
	if err != nil || len(res.Matches) != 1 || res.Matches[0].Line != 3 {
		t.Fatalf("dashes: %+v %v", res, err)
	}
}

// The open scope searches exactly the given files, including non-markdown
// ones, and nothing else.
func TestRunOpenScope(t *testing.T) {
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
	a := write("a.md", "open needle here\n")
	b := write("b.txt", "another open needle\n")
	write("c.md", "closed needle\n")
	res, err := Run(context.Background(), Request{Query: "needle", Scope: Open, Root: dir, Files: []string{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 2 || res.Files != 2 {
		t.Fatalf("open search: %+v", res)
	}
	for _, m := range res.Matches {
		if m.Path != a && m.Path != b {
			t.Fatalf("open search left its files: %+v", res)
		}
	}
	// With no files it finds nothing, not the working directory's lines.
	t.Chdir(dir)
	if res, err := Run(context.Background(), Request{Query: "needle", Scope: Open, Root: dir}); err != nil || len(res.Matches) != 0 {
		t.Fatalf("open search of no files: %+v %v", res, err)
	}
}

// Ignored files are still searched: a directory search covers what the file
// menu lists, matching the file scope's explicit paths, which ripgrep never
// filters by ignore rules.
func TestRunSearchesIgnoredFiles(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("the lawyer rested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A git repository so the ignore rule applies: bare .ignore files are
	// not the case at issue.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), Request{Query: "lawyer", Scope: Dir, Root: dir})
	if err != nil || len(res.Matches) != 1 || res.Files != 1 {
		t.Fatalf("dir search of an ignored file: %+v %v", res, err)
	}
}

// Exclude globs skip matching paths in directory walks but never filter an
// explicitly searched file: Request.Exclude becomes rg -g '!pat' args.
func TestRunExcludeGlobs(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("keep.md", "the lawyer rested\n")
	write(filepath.Join("skipme", "drop.md"), "the lawyer rested\n")
	res, err := Run(context.Background(), Request{Query: "lawyer", Scope: Dir, Root: dir, Exclude: []string{"skipme"}})
	if err != nil || len(res.Matches) != 1 || res.Matches[0].Path != filepath.Join(dir, "keep.md") {
		t.Fatalf("excluded dir still searched: %+v %v", res, err)
	}
	// An explicit file path bypasses globs, like ignores: a file search
	// always covers the opened file.
	res, err = Run(context.Background(), Request{Query: "lawyer", Scope: File, Root: filepath.Join(dir, "skipme", "drop.md"), Exclude: []string{"skipme"}})
	if err != nil || len(res.Matches) != 1 {
		t.Fatalf("file search of an excluded file: %+v %v", res, err)
	}
}
