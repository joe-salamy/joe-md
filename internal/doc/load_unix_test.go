//go:build unix

package doc

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Opening a FIFO for reading waits for a writer, so Load must turn it away
// before it reads.
func TestLoadRejectsFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Load(p)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotFile) {
			t.Errorf("err = %v, want ErrNotFile", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Load blocked on a FIFO")
	}
}
