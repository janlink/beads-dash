// Package term probes the terminal before Bubble Tea owns the TTY: the
// background colour (OSC 11) and whether ambiguous-width glyphs are drawn wide
// (cursor position report). Probes run against the Terminal interface so they
// can be tested with a fake terminal.
package term

import (
	"bytes"
	"image/color"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Budget is the combined time the probes may take.
const Budget = 150 * time.Millisecond

// probeGlyph is an ambiguous-width character; its width is read back from the
// cursor position.
const probeGlyph = "●"

// Terminal is the raw terminal the probes talk to. Read must return
// os.ErrDeadlineExceeded (or any error) once the read deadline passes.
type Terminal interface {
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	SetReadDeadline(t time.Time) error
}

// Options selects the probes and their time budget.
type Options struct {
	// Budget bounds all probes together; zero means Budget.
	Budget time.Duration
	// Background queries the terminal background colour.
	Background bool
	// Ambiguous measures whether ambiguous-width glyphs are drawn wide.
	Ambiguous bool
}

// Result is the probe outcome. Unknown answers keep the fallbacks: dark
// background, narrow ambiguous glyphs.
type Result struct {
	// Interactive is set by callers that opened a real terminal for the probe.
	Interactive     bool
	Dark            bool
	BackgroundKnown bool
	AmbiguousWide   bool
	AmbiguousKnown  bool
}

// Probe queries the terminal within the budget and never fails: whatever does
// not answer in time keeps its fallback.
func Probe(t Terminal, o Options) Result {
	res := Result{Dark: true}
	if !o.Background && !o.Ambiguous {
		return res
	}
	budget := o.Budget
	if budget <= 0 {
		budget = Budget
	}

	var q bytes.Buffer
	if o.Background {
		q.WriteString(ansi.RequestBackgroundColor)
		q.WriteString(ansi.RequestPrimaryDeviceAttributes)
	}
	if o.Ambiguous {
		q.WriteString("\r" + probeGlyph + ansi.RequestCursorPositionReport + "\r\x1b[K")
	}
	if _, err := t.Write(q.Bytes()); err != nil {
		return res
	}

	deadline := time.Now().Add(budget)
	if err := t.SetReadDeadline(deadline); err != nil {
		return res
	}
	defer t.SetReadDeadline(time.Time{}) //nolint:errcheck // best effort reset

	bgDone := !o.Background
	widthDone := !o.Ambiguous
	var pending []byte
	buf := make([]byte, 256)
	for !bgDone || !widthDone {
		n, err := t.Read(buf)
		pending = append(pending, buf[:n]...)
		var replies []reply
		replies, pending = scan(pending)
		for _, r := range replies {
			switch r.kind {
			case replyBackground:
				if !bgDone {
					res.Dark, res.BackgroundKnown, bgDone = isDark(r.colour), true, true
				}
			case replyDeviceAttributes:
				bgDone = true
			case replyCursor:
				if !widthDone {
					res.AmbiguousWide, res.AmbiguousKnown, widthDone = r.col-1 >= 2, true, true
				}
			}
		}
		if err != nil {
			break
		}
	}
	return res
}

func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return lum/0xffff < 0.5
}

type replyKind int

const (
	replyBackground replyKind = iota
	replyDeviceAttributes
	replyCursor
)

type reply struct {
	kind   replyKind
	colour color.Color
	col    int
}

// scan extracts terminal replies from buf and returns them with the bytes that
// may still be the start of an incomplete reply.
func scan(buf []byte) ([]reply, []byte) {
	var out []reply
	for {
		i := bytes.IndexByte(buf, 0x1b)
		if i < 0 {
			return out, nil
		}
		buf = buf[i:]
		r, n, complete := parseReply(buf)
		if !complete {
			return out, buf
		}
		if r != nil {
			out = append(out, *r)
		}
		buf = buf[n:]
	}
}

// parseReply parses one escape sequence at the start of buf. It returns the
// reply (nil for sequences bdash does not care about), the bytes consumed and
// false when buf ends inside the sequence.
func parseReply(buf []byte) (*reply, int, bool) {
	if len(buf) < 2 {
		return nil, 0, false
	}
	switch buf[1] {
	case ']':
		end, termLen := oscEnd(buf)
		if end < 0 {
			return nil, 0, false
		}
		body := string(buf[2:end])
		n := end + termLen
		if len(body) > 3 && body[:3] == "11;" {
			if c := ansi.XParseColor(body[3:]); c != nil {
				return &reply{kind: replyBackground, colour: c}, n, true
			}
		}
		return nil, n, true
	case '[':
		for j := 2; j < len(buf); j++ {
			if buf[j] >= 0x40 && buf[j] <= 0x7e {
				return csiReply(buf[2:j], buf[j]), j + 1, true
			}
		}
		return nil, 0, false
	default:
		return nil, 2, true
	}
}

func oscEnd(buf []byte) (end, termLen int) {
	for j := 2; j < len(buf); j++ {
		switch {
		case buf[j] == 0x07:
			return j, 1
		case buf[j] == 0x1b && j+1 < len(buf) && buf[j+1] == '\\':
			return j, 2
		case buf[j] == 0x1b && j+1 >= len(buf):
			return -1, 0
		}
	}
	return -1, 0
}

func csiReply(params []byte, final byte) *reply {
	switch final {
	case 'c':
		if len(params) > 0 && params[0] == '?' {
			return &reply{kind: replyDeviceAttributes}
		}
	case 'R':
		var row, col int
		if parseCursor(params, &row, &col) {
			return &reply{kind: replyCursor, col: col}
		}
	}
	return nil
}

func parseCursor(params []byte, row, col *int) bool {
	semi := bytes.IndexByte(params, ';')
	if semi < 0 {
		return false
	}
	var ok bool
	if *row, ok = atoi(params[:semi]); !ok {
		return false
	}
	*col, ok = atoi(params[semi+1:])
	return ok
}

func atoi(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
