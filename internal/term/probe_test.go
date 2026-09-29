package term_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/term"
)

const (
	testBudget = 60 * time.Millisecond
	fastLimit  = 10 * testBudget
)

func probe(f *term.FakeTerminal, bg, amb bool) (term.Result, time.Duration) {
	start := time.Now()
	res := term.Probe(f, term.Options{Budget: testBudget, Background: bg, Ambiguous: amb})
	return res, time.Since(start)
}

func TestProbeBackground(t *testing.T) {
	tests := []struct {
		name     string
		colour   string
		wantDark bool
	}{
		{"black", "rgb:0000/0000/0000", true},
		{"white", "rgb:ffff/ffff/ffff", false},
		{"two digit", "rgb:1e/1e/2e", true},
		{"light grey", "rgb:eeee/f0f0/f5f5", false},
		{"rgba", "rgba:0000/0000/0000/ffff", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &term.FakeTerminal{Background: tt.colour, DeviceAttributes: true}
			res, took := probe(f, true, false)
			if !res.BackgroundKnown || res.Dark != tt.wantDark {
				t.Errorf("got %+v, want dark=%v known", res, tt.wantDark)
			}
			if took > fastLimit {
				t.Errorf("took %v, want a prompt return", took)
			}
		})
	}
}

func TestProbeAmbiguousWidth(t *testing.T) {
	for _, tt := range []struct {
		width int
		wide  bool
	}{{1, false}, {2, true}} {
		f := &term.FakeTerminal{AmbiguousWidth: tt.width}
		res, took := probe(f, false, true)
		if !res.AmbiguousKnown || res.AmbiguousWide != tt.wide {
			t.Errorf("width %d: got %+v", tt.width, res)
		}
		if took > fastLimit {
			t.Errorf("width %d took %v", tt.width, took)
		}
	}
}

func TestProbeSilentTerminalFallsBackAtBudget(t *testing.T) {
	f := &term.FakeTerminal{}
	res, took := probe(f, true, true)
	if res.BackgroundKnown || res.AmbiguousKnown || !res.Dark || res.AmbiguousWide {
		t.Errorf("got %+v, want dark/narrow fallbacks", res)
	}
	if took < testBudget {
		t.Errorf("took %v, want about %v", took, testBudget)
	}
}

func TestProbeUnsupportedOSC11ReturnsOnDeviceAttributes(t *testing.T) {
	f := &term.FakeTerminal{DeviceAttributes: true, AmbiguousWidth: 1}
	res, took := probe(f, true, true)
	if res.BackgroundKnown || !res.Dark {
		t.Errorf("got %+v, want unknown dark background", res)
	}
	if !res.AmbiguousKnown || res.AmbiguousWide {
		t.Errorf("got %+v, want known narrow", res)
	}
	if took > fastLimit {
		t.Errorf("took %v, want return without waiting for the budget", took)
	}
}

func TestProbePartialAnswerKeepsWhatArrived(t *testing.T) {
	f := &term.FakeTerminal{Background: "rgb:ffff/ffff/ffff", DeviceAttributes: true}
	res, took := probe(f, true, true)
	if !res.BackgroundKnown || res.Dark {
		t.Errorf("background: got %+v", res)
	}
	if res.AmbiguousKnown || res.AmbiguousWide {
		t.Errorf("ambiguous should fall back to narrow: %+v", res)
	}
	if took < testBudget {
		t.Errorf("took %v, want to wait out the budget for the missing cursor report", took)
	}
}

func TestProbeSurvivesFragmentedAndNoisyReplies(t *testing.T) {
	f := &term.FakeTerminal{
		Background:       "rgb:ffff/ffff/ffff",
		DeviceAttributes: true,
		AmbiguousWidth:   2,
		Chunk:            1,
		Junk:             "\x1b[<0;1;1M\x1b]10;rgb:0/0/0\x07xyz",
	}
	res, _ := probe(f, true, true)
	want := term.Result{Dark: false, BackgroundKnown: true, AmbiguousWide: true, AmbiguousKnown: true}
	if res != want {
		t.Errorf("got %+v, want %+v", res, want)
	}
}

func TestProbeOnlyWritesRequestedQueries(t *testing.T) {
	f := &term.FakeTerminal{Background: "rgb:0/0/0", DeviceAttributes: true}
	probe(f, true, false)
	if w := f.Written(); !strings.Contains(w, ansi.RequestBackgroundColor) || strings.Contains(w, ansi.RequestCursorPositionReport) {
		t.Errorf("written %q", w)
	}
	f = &term.FakeTerminal{AmbiguousWidth: 1}
	probe(f, false, true)
	if w := f.Written(); strings.Contains(w, ansi.RequestBackgroundColor) || !strings.Contains(w, ansi.RequestCursorPositionReport) {
		t.Errorf("written %q", w)
	}
}

func TestProbeWithNothingRequestedTouchesNothing(t *testing.T) {
	f := &term.FakeTerminal{}
	res := term.Probe(f, term.Options{})
	if f.Written() != "" || !res.Dark {
		t.Errorf("written %q result %+v", f.Written(), res)
	}
}

func TestEnableColorSchemeReportingResetsOnce(t *testing.T) {
	var sb strings.Builder
	reset := term.EnableColorSchemeReporting(&sb)
	if sb.String() != ansi.SetModeLightDark {
		t.Fatalf("enable wrote %q", sb.String())
	}
	reset()
	reset()
	if want := ansi.SetModeLightDark + ansi.ResetModeLightDark; sb.String() != want {
		t.Errorf("wrote %q, want %q", sb.String(), want)
	}
}
