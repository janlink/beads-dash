package term

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestCloseDoesNotHangWithUnreadInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	tty, err := newTTY(r, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	// More chunks than the channel holds, never read.
	for range 3 * cap(tty.chunks) {
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	closed := make(chan error, 1)
	go func() { closed <- tty.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close hangs while the pump is blocked on unread input")
	}
}
