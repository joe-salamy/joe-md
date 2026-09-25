package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestResult(t *testing.T) {
	for _, tc := range []struct {
		content string
		want    int
	}{
		{"157\n", 157},
		{"", 42},     // plugin never ran
		{"junk", 42}, // unreadable
		{"0\n", 42},
	} {
		f := filepath.Join(t.TempDir(), "cursor")
		if err := os.WriteFile(f, []byte(tc.content), 0o600); err != nil {
			t.Fatal(err)
		}
		s := &Session{Line: 42, cursorFile: f}
		if got := s.Result(); got != tc.want {
			t.Errorf("Result(%q) = %d, want %d", tc.content, got, tc.want)
		}
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("Result(%q) left the cursor file behind", tc.content)
		}
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MICRO_CONFIG_HOME", dir)
	dst := filepath.Join(dir, "plug", "joemd", "joemd.lua")
	for range 2 { // installing twice is harmless
		if err := Install(); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(dst); err != nil || !bytes.Equal(b, plugin) {
			t.Fatalf("plugin not installed at %s: %v", dst, err)
		}
	}
	// An outdated copy is replaced.
	os.WriteFile(dst, []byte("old"), 0o644)
	if err := Install(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); !bytes.Equal(b, plugin) {
		t.Error("outdated plugin was not replaced")
	}
}
