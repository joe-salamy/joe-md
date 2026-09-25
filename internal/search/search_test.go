package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
