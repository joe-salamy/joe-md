// Package editor runs micro on a document and reports back the line its
// cursor was on when micro exited.
package editor

import (
	"bytes"
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// plugin is the micro plugin that writes the cursor line to
// $JOE_MD_CURSOR_FILE after every event. It is inert when micro is started
// by anything other than joe-md.
//
//go:embed joemd.lua
var plugin []byte

// Session is one run of micro on one file.
type Session struct {
	Line       int // 1-based line micro was opened on
	cursorFile string
}

// Command returns the micro command that opens absolute path at 1-based line. The
// command must be run (e.g. with tea.ExecProcess) before calling Result.
// A non-nil command may come with a non-nil error: micro can still run, but
// the plugin could not be installed so the cursor line will not come back.
func Command(path string, line int) (*exec.Cmd, *Session, error) {
	bin, err := exec.LookPath("micro")
	if err != nil {
		return nil, nil, errors.New("micro not found on $PATH")
	}
	f, err := os.CreateTemp("", "joe-md-cursor-*")
	if err != nil {
		return nil, nil, err
	}
	f.Close()

	cmd := exec.Command(bin, "+"+strconv.Itoa(line), path)
	cmd.Env = append(os.Environ(), "JOE_MD_CURSOR_FILE="+f.Name(), "JOE_MD_FILE="+path)
	return cmd, &Session{Line: line, cursorFile: f.Name()}, install()
}

// Result returns the 1-based line the cursor was on when micro exited, or
// the line micro was opened on if the plugin reported nothing. It removes
// the session's temporary file.
func (s *Session) Result() int {
	defer os.Remove(s.cursorFile)
	b, err := os.ReadFile(s.cursorFile)
	if err != nil {
		return s.Line
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 1 {
		return s.Line
	}
	return n
}

// install writes the plugin into micro's config directory, if it is missing
// or out of date.
func install() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, "plug", "joemd", "joemd.lua")
	if cur, err := os.ReadFile(dst); err == nil && bytes.Equal(cur, plugin) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, plugin, 0o644)
}

// configDir mirrors micro's own lookup of its configuration directory.
func configDir() (string, error) {
	if d := os.Getenv("MICRO_CONFIG_HOME"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "micro"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "micro"), nil
}
