package screens_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/screens"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func session(raw string) bd.Session {
	v, _ := bd.CheckVersion(raw)
	return bd.Session{Version: v, Statuses: model.BuiltinStatuses()}
}

func withWorkspace(s bd.Session) bd.Session {
	s.Workspace = bd.Workspace{Path: "/work/demo/.beads"}
	return s
}

type scenario struct {
	name string
	in   screens.Input
	kind screens.Kind
	step screens.Step
}

func scenarios() []scenario {
	_, refused := bd.CheckVersion("1.2.1")
	_, tooOld := bd.CheckVersion("1.1.0")
	transient := &bd.Error{Class: bd.ClassTransient, Command: "list", ExitCode: 1, Stderr: "Error: database is locked\nretry later"}
	return []scenario{
		{"bd-missing", screens.Input{Err: &bd.Error{Class: bd.ClassBdMissing, Command: "version", Message: "exec: \"bd\": executable file not found in $PATH"}, Dir: "/work/demo"}, screens.BdMissing, screens.StepBd},
		{"bd-refused", screens.Input{Err: refused, Session: session("1.2.1"), BdPath: "/usr/bin/bd"}, screens.BdRefused, screens.StepVersion},
		{"bd-too-old", screens.Input{Err: tooOld, Session: session("1.1.0"), BdPath: "/usr/bin/bd"}, screens.BdTooOld, screens.StepVersion},
		{"schema", screens.Input{Err: &bd.Error{Class: bd.ClassUnsupported, Command: "where", Message: "json schema 2"}, Session: session("1.2.3"), BdashVersion: "0.1.0"}, screens.SchemaMismatch, screens.StepSchema},
		{"not-workspace", screens.Input{Err: &bd.Error{Class: bd.ClassNotWorkspace, Command: "where", ExitCode: 1, Stderr: "no beads database found"}, Session: session("1.2.3"), Dir: "/work/elsewhere"}, screens.NotWorkspace, screens.StepWorkspace},
		{"unreadable", screens.Input{Err: transient, Session: withWorkspace(session("1.2.3"))}, screens.Unreadable, screens.StepSnapshot},
		{"first-snapshot", screens.Input{Err: transient, Session: withWorkspace(session("1.2.3")), FirstSnapshot: true}, screens.FirstSnapshot, screens.StepSnapshot},
		{"vanished", screens.Input{Err: &bd.Error{Class: bd.ClassNotWorkspace, Command: "list"}, Session: withWorkspace(session("1.2.3")), Vanished: true, Dir: "/work/demo"}, screens.Vanished, screens.StepWorkspace},
	}
}

func TestDiagnoseAttributesTheFailedStep(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			r := screens.Diagnose(sc.in)
			if r.Kind != sc.kind {
				t.Errorf("kind %v, want %v", r.Kind, sc.kind)
			}
			for i, c := range r.Checks {
				want := screens.NotRun
				switch {
				case i < int(sc.step):
					want = screens.Passed
				case i == int(sc.step):
					want = screens.Failed
				}
				if c.State != want {
					t.Errorf("check %s: %v, want %v", c.Name, c.State, want)
				}
			}
			if r.Title == "" || r.What == "" {
				t.Error("report without wording")
			}
		})
	}
}

func TestDiagnoseTransientWithoutWorkspaceBlamesTheWorkspaceStep(t *testing.T) {
	r := screens.Diagnose(screens.Input{Err: &bd.Error{Class: bd.ClassTimeout, Command: "where"}, Session: session("1.2.3")})
	if r.Checks[screens.StepWorkspace].State != screens.Failed {
		t.Errorf("checks %+v", r.Checks)
	}
}

func TestFixesFollowHowBdWasInstalled(t *testing.T) {
	s := session("1.1.0")
	s.Version.Build = "Homebrew"
	_, err := bd.CheckVersion("1.1.0")
	r := screens.Diagnose(screens.Input{Err: err, Session: s})
	if len(r.Fixes) == 0 || !strings.HasPrefix(r.Fixes[0].Cmd, "brew upgrade") {
		t.Errorf("fixes %+v", r.Fixes)
	}
}

func TestRenderGoldens(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	hints := keys.Default().Hints(keys.Startup)
	for _, sc := range scenarios() {
		for _, size := range [][2]int{{80, 24}, {60, 16}} {
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, size[0], size[1]), func(t *testing.T) {
				out := screens.Render(screens.View{Look: l, Hints: hints, Cols: size[0], Rows: size[1]}, screens.Diagnose(sc.in))
				if len(out) != size[1] {
					t.Fatalf("%d lines", len(out))
				}
				for _, line := range out {
					if w := ansi.StringWidth(line); w != size[0] {
						t.Fatalf("line width %d: %q", w, line)
					}
				}
				testgolden.Equal(t, ansi.Strip(strings.Join(out, "\n")))
			})
		}
	}
}

func TestRenderRetryCountdown(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	sc := scenarios()[0]
	out := screens.Render(screens.View{Look: l, Cols: 80, Rows: 24, RetryIn: 4500e6}, screens.Diagnose(sc.in))
	if !strings.Contains(ansi.Strip(strings.Join(out, "\n")), "rechecking in 5s") {
		t.Error("no countdown")
	}
}

func TestTooSmallAndNoticeAndEmpty(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	got := ansi.Strip(strings.Join(screens.TooSmall(l, 50, 10, 60, 16), "\n"))
	if !strings.Contains(got, "Terminal too small") || !strings.Contains(got, "needs 60x16") {
		t.Errorf("too small: %q", got)
	}
	row := ansi.Strip(screens.NoticeRow(l, screens.Notice{Text: "refresh failed", Warn: true, Keys: []keys.Hint{{Key: "r", Desc: "retry"}}}, 60))
	if ansi.StringWidth(row) != 60 || !strings.HasSuffix(row, "r retry") {
		t.Errorf("notice %q", row)
	}
	for _, e := range []screens.Empty{screens.EmptyWorkspace("demo", true), screens.EmptyReady, screens.EmptyBlocked, screens.EmptyScope, screens.EmptyMemories, screens.EmptyGraph} {
		out := screens.RenderEmpty(l, e, 60, 10)
		if len(out) != 10 || !strings.Contains(ansi.Strip(strings.Join(out, "\n")), e.Title) {
			t.Errorf("empty %q", e.Title)
		}
	}
}

func TestDiagnoseKeepsRawOutput(t *testing.T) {
	r := screens.Diagnose(screens.Input{Err: errors.New("plain failure"), Session: withWorkspace(session("1.2.3"))})
	if len(r.Raw) != 1 || r.Raw[0] != "plain failure" {
		t.Errorf("raw %v", r.Raw)
	}
}

func TestEmptyWorkspaceWording(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	e := screens.EmptyWorkspace("demo", true)
	got := ansi.Strip(strings.Join(screens.RenderEmpty(l, e, 100, 12), "\n"))
	for _, want := range []string{
		"No issues in demo yet.",
		"bdash picks up new issues within about 2 s, whoever creates them.",
		"n new issue",
		`bd create "First task" -t task`,
		"bd quickstart",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty workspace lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(strings.Join(hintKeys(screens.EmptyWorkspace("demo", false).Hints), " "), "new issue") {
		t.Error("the n hint is only shown while n is bound")
	}
}

func hintKeys(hs []keys.Hint) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.Key + " " + h.Desc
	}
	return out
}

func TestTextRendersTheChecklist(t *testing.T) {
	r := screens.Diagnose(screens.Input{Err: &bd.Error{Class: bd.ClassBdMissing, Command: "bd", Message: "not found"}})
	got := screens.Text(r)
	for _, want := range []string{r.Title, "x bd", "- snapshot", "What to do", "$ "} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, screens.SameReport) {
		t.Errorf("plain text must have no styling and not point at itself:\n%q", got)
	}
}

func TestStartupScreenPointsAtTheVersionReport(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	r := screens.Diagnose(screens.Input{Err: &bd.Error{Class: bd.ClassBdMissing, Command: "bd", Message: "not found"}})
	out := ansi.Strip(strings.Join(screens.Render(screens.View{Look: l, Cols: 100, Rows: 40}, r), "\n"))
	if !strings.Contains(out, "same report: bdash -v") {
		t.Errorf("no pointer to bdash -v:\n%s", out)
	}
}
