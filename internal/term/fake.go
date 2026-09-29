package term

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// FakeTerminal is a scripted Terminal for tests. It answers the queries the
// probes write according to its fields, in the order they were written.
type FakeTerminal struct {
	// Background is the OSC 11 payload to answer with, e.g.
	// "rgb:0000/0000/0000"; empty means OSC 11 is unsupported.
	Background string
	// DeviceAttributes answers the primary device attributes query.
	DeviceAttributes bool
	// AmbiguousWidth is the width the terminal draws the probe glyph with; 0
	// means no cursor position report is sent.
	AmbiguousWidth int
	// Chunk delivers replies at most this many bytes per Read; 0 means all.
	Chunk int
	// Junk is prepended to the first reply batch.
	Junk string

	mu       sync.Mutex
	written  bytes.Buffer
	inbox    []byte
	deadline time.Time
	junked   bool
}

// Written returns everything the probes wrote.
func (f *FakeTerminal) Written() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.String()
}

func (f *FakeTerminal) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written.Write(p)
	s := string(p)
	for len(s) > 0 {
		switch {
		case strings.HasPrefix(s, ansi.RequestBackgroundColor):
			s = s[len(ansi.RequestBackgroundColor):]
			if f.Background != "" {
				f.inbox = append(f.inbox, "\x1b]11;"+f.Background+"\x1b\\"...)
			}
		case strings.HasPrefix(s, ansi.RequestPrimaryDeviceAttributes):
			s = s[len(ansi.RequestPrimaryDeviceAttributes):]
			if f.DeviceAttributes {
				f.inbox = append(f.inbox, "\x1b[?62;22c"...)
			}
		case strings.HasPrefix(s, ansi.RequestCursorPositionReport):
			s = s[len(ansi.RequestCursorPositionReport):]
			if f.AmbiguousWidth > 0 {
				f.inbox = append(f.inbox, "\x1b[5;"+strconv.Itoa(1+f.AmbiguousWidth)+"R"...)
			}
		default:
			s = s[1:]
		}
	}
	if !f.junked && len(f.inbox) > 0 && f.Junk != "" {
		f.inbox = append([]byte(f.Junk), f.inbox...)
		f.junked = true
	}
	return len(p), nil
}

func (f *FakeTerminal) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deadline = t
	return nil
}

func (f *FakeTerminal) Read(p []byte) (int, error) {
	f.mu.Lock()
	if len(f.inbox) > 0 {
		n := len(f.inbox)
		if f.Chunk > 0 {
			n = min(n, f.Chunk)
		}
		n = copy(p, f.inbox[:n])
		f.inbox = f.inbox[n:]
		f.mu.Unlock()
		return n, nil
	}
	deadline := f.deadline
	f.mu.Unlock()
	if deadline.IsZero() {
		return 0, io.EOF
	}
	time.Sleep(time.Until(deadline))
	return 0, os.ErrDeadlineExceeded
}
