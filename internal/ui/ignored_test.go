package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Ignored markdown shows by default and hides on alt+.; HideIgnored starts
// hidden. Needs git for check-ignore.
func TestToggleIgnoredSmoke(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	must := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("keep.md", "# keep\n")
	must("skip.md", "# skip\n")
	must(".gitignore", "skip.md\n")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("add", "keep.md", ".gitignore")
	a := newTestApp(t, filepath.Join(dir, "keep.md"))
	press(a, "o")
	if got := menuNames(a); got != ".. keep.md skip.md" {
		t.Fatalf("default shows ignored: %q", got)
	}
	press(a, "alt+.")
	if got := menuNames(a); got != ".. keep.md" {
		t.Fatalf("alt+. hides ignored: %q", got)
	}
	press(a, "alt+.")
	if got := menuNames(a); got != ".. keep.md skip.md" {
		t.Fatalf("alt+. again shows ignored: %q", got)
	}
	press(a, "esc")
	b := New(nil, Options{Style: "notty", MaxWrap: 80, MenuDir: dir, HideIgnored: true})
	b.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if b.menu == nil {
		t.Fatal("start-up menu did not open")
	}
	if got := menuNames(b); got != ".. keep.md" {
		t.Fatalf("HideIgnored starts hidden: %q", got)
	}
}
