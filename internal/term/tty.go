package term

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	xterm "github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
)

// ErrNotTerminal is returned by Open when stdin or stdout is not a terminal.
var ErrNotTerminal = errors.New("term: stdin and stdout must be terminals")

// TTY is the real terminal on stdin/stdout in raw mode. Close hands the
// terminal back exactly as found, so Bubble Tea can take it over.
type TTY struct {
	out     io.Writer
	reader  cancelreader.CancelReader
	fd      uintptr
	state   *xterm.State
	chunks  chan []byte
	done    chan struct{}
	stop    chan struct{}
	pending []byte

	closeOnce sync.Once
	mu        sync.Mutex
	deadline  time.Time
}

// Open puts the controlling terminal into raw mode and starts reading input.
func Open() (*TTY, error) {
	in, out := os.Stdin, os.Stdout
	if !xterm.IsTerminal(in.Fd()) || !xterm.IsTerminal(out.Fd()) {
		return nil, ErrNotTerminal
	}
	state, err := xterm.MakeRaw(in.Fd())
	if err != nil {
		return nil, err
	}
	t, err := newTTY(in, out, state)
	if err != nil {
		_ = xterm.Restore(in.Fd(), state)
		return nil, err
	}
	return t, nil
}

func newTTY(in *os.File, out io.Writer, state *xterm.State) (*TTY, error) {
	reader, err := cancelreader.NewReader(in)
	if err != nil {
		return nil, err
	}
	t := &TTY{
		out:    out,
		reader: reader,
		fd:     in.Fd(),
		state:  state,
		chunks: make(chan []byte, 16),
		done:   make(chan struct{}),
		stop:   make(chan struct{}),
	}
	go t.pump()
	return t, nil
}

func (t *TTY) pump() {
	defer close(t.done)
	defer close(t.chunks)
	buf := make([]byte, 256)
	for {
		n, err := t.reader.Read(buf)
		if n > 0 {
			select {
			case t.chunks <- append([]byte(nil), buf[:n]...):
			case <-t.stop:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (t *TTY) Write(p []byte) (int, error) { return t.out.Write(p) }

func (t *TTY) SetReadDeadline(d time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deadline = d
	return nil
}

func (t *TTY) Read(p []byte) (int, error) {
	if len(t.pending) > 0 {
		n := copy(p, t.pending)
		t.pending = t.pending[n:]
		return n, nil
	}
	t.mu.Lock()
	deadline := t.deadline
	t.mu.Unlock()

	var expired <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case chunk, ok := <-t.chunks:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, chunk)
		t.pending = chunk[n:]
		return n, nil
	case <-expired:
		return 0, os.ErrDeadlineExceeded
	}
}

// Close stops reading and restores the terminal mode.
func (t *TTY) Close() error {
	t.closeOnce.Do(func() { close(t.stop) })
	t.reader.Cancel()
	<-t.done
	_ = t.reader.Close()
	if t.state == nil {
		return nil
	}
	return xterm.Restore(t.fd, t.state)
}

// SetColorSchemeReporting turns terminal light/dark reports (mode 2031) on or
// off. Terminals without support ignore both.
func SetColorSchemeReporting(w io.Writer, on bool) error {
	seq := ansi.ResetModeLightDark
	if on {
		seq = ansi.SetModeLightDark
	}
	_, err := io.WriteString(w, seq)
	return err
}
