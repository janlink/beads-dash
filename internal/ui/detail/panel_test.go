package detail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

var now = uitest.T0

func fixture() *model.Snapshot {
	long := make([]string, 30)
	for i := range long {
		long[i] = fmt.Sprintf("line %d of the description", i+1)
	}
	issues := []model.Issue{
		{
			ID: "d-1", Title: "Epic with children", Status: "open", IssueType: "epic", Priority: 1, Assignee: "alice",
			Description: strings.Join(long, "\n\n"), Design: "Use **two** phases.", Labels: []string{"backend", "api", "urgent-fix", "q4", "tech-debt", "security"},
			CreatedAt: now.Add(-72 * time.Hour), UpdatedAt: now.Add(-time.Hour), CreatedBy: "bob",
		},
		{ID: "d-1.1", Title: "First child", Status: "closed", IssueType: "task", Priority: 2, Parent: "d-1", CreatedAt: now.Add(-48 * time.Hour), ClosedAt: now.Add(-time.Hour), CloseReason: "done"},
		{
			ID: "d-1.2", Title: "Second child", Status: "open", IssueType: "task", Priority: 1, Parent: "d-1", Assignee: "carol", CreatedAt: now.Add(-24 * time.Hour),
			Dependencies: []model.Edge{{From: "d-1.2", To: "d-2", Type: "blocks"}},
		},
		{ID: "d-2", Title: "Blocker", Status: "in_progress", IssueType: "bug", Priority: 0, CreatedAt: now.Add(-24 * time.Hour)},
		{ID: "d-3", Title: "Plain", Status: "deferred", IssueType: "task", Priority: 3, CreatedAt: now.Add(-time.Hour)},
	}
	return model.NewSnapshot(issues, model.Readiness{Blocked: map[string][]string{"d-1.2": {"d-2"}}}, now)
}

func input(t testing.TB, snap *model.Snapshot, id string, f Frame, w, h int) Input {
	t.Helper()
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	r := rows.New(l)
	r.Bind(snap, model.BuiltinStatuses())
	return Input{Look: l, Snap: snap, Statuses: model.BuiltinStatuses(), Rows: r, ID: id, Now: now, Frame: f, W: w, H: h}
}

func text(lines []string) string { return ansi.Strip(strings.Join(lines, "\n")) }

func TestRenderSizesAndHeader(t *testing.T) {
	snap := fixture()
	for _, f := range []Frame{Side, Bottom, Overlay} {
		for _, size := range [][2]int{{60, 14}, {80, 10}, {90, 30}} {
			in := input(t, snap, "d-1.2", f, size[0], size[1])
			out := New().Render(in)
			if len(out) != size[1] {
				t.Fatalf("%v %v: %d lines", f, size, len(out))
			}
			for i, s := range out {
				if w := ansi.StringWidth(s); w != size[0] {
					t.Fatalf("%v %v line %d is %d wide: %q", f, size, i, w, ansi.Strip(s))
				}
			}
		}
	}
	got := text(New().Render(input(t, snap, "d-1.2", Overlay, 100, 30)))
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[0], "d-1.2") || !strings.Contains(lines[0], "Blocked (open)") || !strings.Contains(lines[0], "P1") || !strings.Contains(lines[0], "@carol") {
		t.Errorf("header line 1: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Second child") {
		t.Errorf("header line 2: %q", lines[1])
	}
}

func TestHeaderShowsProgressForContainers(t *testing.T) {
	got := text(New().Render(input(t, fixture(), "d-1", Overlay, 100, 30)))
	if first := strings.Split(got, "\n")[0]; !strings.Contains(first, "1/2") || !strings.Contains(first, "epic") {
		t.Errorf("header: %q", first)
	}
}

func TestDefaultSectionsAndOmissions(t *testing.T) {
	p := New()
	got := text(p.Render(input(t, fixture(), "d-3", Overlay, 100, 40)))
	for _, want := range []string{"* Description  none", "* Dependencies  none", "* Children  none", "> Details"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if got := text(New().Render(input(t, fixture(), "d-1", Overlay, 100, 40))); !strings.Contains(got, "v Description  markdown") {
		t.Errorf("open Description header:\n%s", got)
	}
	for _, absent := range []string{"Design", "Acceptance", "Notes"} {
		if strings.Contains(got, absent) {
			t.Errorf("empty section %q is shown", absent)
		}
	}
}

func TestDescriptionCapAndExpand(t *testing.T) {
	p := New()
	got := text(p.Render(input(t, fixture(), "d-1", Overlay, 100, 60)))
	if !strings.Contains(got, "> 18 more lines") && !strings.Contains(got, "more lines") {
		t.Fatalf("no cap marker:\n%s", got)
	}
	if strings.Contains(got, "line 25 of") {
		t.Error("lines past the cap are shown")
	}
	p.Expand()
	got = text(p.Render(input(t, fixture(), "d-1", Overlay, 100, 80)))
	if !strings.Contains(got, "line 30 of") || strings.Contains(got, "more lines") {
		t.Errorf("Expand did not lift the cap:\n%s", got)
	}
}

func TestSectionKeys(t *testing.T) {
	p := New()
	snap := fixture()
	render := func() string { return text(p.Render(input(t, snap, "d-1", Overlay, 100, 30))) }
	render()
	for _, want := range []Section{Design, Children, Details, Details} {
		p.Next()
		if p.Cursor() != want {
			t.Fatalf("cursor %v, want %v", p.Cursor(), want)
		}
	}
	p.Prev()
	if p.Cursor() != Children {
		t.Fatalf("Prev: cursor %v", p.Cursor())
	}
	p.Collapse()
	if got := render(); !strings.Contains(got, "> Children  1/2 closed") {
		t.Errorf("collapsed Children has no summary:\n%s", got)
	}
	p.Toggle()
	if !p.Open(Children) {
		t.Error("Toggle did not reopen")
	}
	p.Toggle()
	if p.Open(Children) {
		t.Error("Toggle did not close an open section")
	}
	p.ToggleAll()
	if p.Open(Details) || p.Open(Description) {
		t.Error("close all left sections open")
	}
	p.ToggleAll()
	for s := range sectionCount {
		if !p.Open(s) {
			t.Errorf("section %v closed after open all", s)
		}
	}
	p.ToggleAll()
	for range 10 {
		p.Prev()
	}
	if p.Cursor() != Description {
		t.Errorf("Prev stops at %v", p.Cursor())
	}
}

func TestSectionStateSurvivesIssueChange(t *testing.T) {
	p := New()
	snap := fixture()
	p.Render(input(t, snap, "d-1", Overlay, 100, 30))
	p.Next()
	p.Next()
	p.Collapse()
	p.Render(input(t, snap, "d-1.2", Overlay, 100, 30))
	if p.Open(Children) {
		t.Error("Children reopened on the next issue")
	}
	if p.Cursor() != Dependencies && p.Cursor() != Description {
		t.Errorf("cursor %v", p.Cursor())
	}
	p.Render(input(t, snap, "d-1", Overlay, 100, 30))
	if p.Cursor() != Description || p.Scroll() != 0 {
		t.Errorf("cursor %v scroll %d not reset", p.Cursor(), p.Scroll())
	}
}

func TestSourceToggle(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1", Overlay, 100, 60)
	p.Render(in)
	p.Next()
	p.Expand()
	settle(p, in)
	before := text(p.Render(in))
	p.ToggleSource()
	after := text(p.Render(in))
	if !strings.Contains(before, "Use two phases.") || !strings.Contains(after, "Use **two** phases.") {
		t.Errorf("design markdown/source:\n%s\n---\n%s", before, after)
	}
	if !p.Source() {
		t.Error("Source() false after toggle")
	}
}

func TestChangedLine(t *testing.T) {
	in := input(t, fixture(), "d-3", Overlay, 100, 30)
	in.Events = []model.Event{
		{Kind: model.KindPriorityChanged, Detail: "2→1", Time: now.Add(-6 * time.Second)},
		{Kind: model.KindCommented, Time: now.Add(-4 * time.Second), Actor: "alice"},
	}
	got := text(New().Render(in))
	if !strings.Contains(got, "Changed  priority 2→1 · commented · 4 s ago by alice") {
		t.Errorf("no changed line:\n%s", got)
	}
	in.Events = nil
	if strings.Contains(text(New().Render(in)), "Changed") {
		t.Error("changed line without events")
	}
}

func TestDependenciesAndChildren(t *testing.T) {
	got := text(New().Render(input(t, fixture(), "d-1.2", Overlay, 100, 30)))
	for _, want := range []string{"parent", "d-1 Epic with children", "waits on 1 (1 open)", "d-2 Blocker", "focus graph"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	got = text(New().Render(input(t, fixture(), "d-1", Overlay, 100, 40)))
	for _, want := range []string{"1/2 closed", "d-1.2 Second child", "x 1 closed"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	got = text(New().Render(input(t, fixture(), "d-2", Overlay, 100, 30)))
	if !strings.Contains(got, "holds up 1") || !strings.Contains(got, "d-1.2 Second child") {
		t.Errorf("holds up:\n%s", got)
	}
}

func TestDetailsSectionAndLabelOverflow(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1", Overlay, 60, 60)
	p.Render(in)
	for range 8 {
		p.Next()
	}
	if p.Cursor() != Details {
		t.Fatalf("cursor %v", p.Cursor())
	}
	p.Expand()
	got := text(p.Render(in))
	for _, want := range []string{"labels", "+", "created", "2026-09-26 12:00", "3 d ago", "bob"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestNoIssue(t *testing.T) {
	out := New().Render(input(t, fixture(), "", Bottom, 80, 10))
	if len(out) != 10 || !strings.Contains(text(out), "No issue selected.") {
		t.Errorf("empty panel:\n%s", text(out))
	}
}

func settle(p *Panel, in Input) {
	for _, j := range p.Plan(in) {
		p.Apply(j.Run())
	}
}

func TestMarkdownArrivesAsynchronously(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1", Overlay, 100, 60)
	p.Render(in)
	p.Next()
	p.Expand()
	pending := text(p.Render(in))
	if !strings.Contains(pending, "Use **two** phases.") {
		t.Fatalf("until the renderer answers the source is shown wrapped:\n%s", pending)
	}
	jobs := p.Plan(in)
	if len(jobs) != 2 {
		t.Fatalf("planned %d jobs, want description and design", len(jobs))
	}
	if again := p.Plan(in); len(again) != 0 {
		t.Errorf("a job in flight is planned once, got %d more", len(again))
	}
	for _, j := range jobs {
		p.Apply(j.Run())
	}
	if done := text(p.Render(in)); !strings.Contains(done, "Use two phases.") {
		t.Errorf("rendered markdown missing:\n%s", done)
	}
	if again := p.Plan(in); len(again) != 0 {
		t.Errorf("finished renderings are cached, got %d jobs", len(again))
	}
	in.Gen++
	if again := p.Plan(in); len(again) != 2 {
		t.Errorf("a new look renders again, got %d jobs", len(again))
	}
}

func TestOnlyDescriptionIsCappedAndInputIsBounded(t *testing.T) {
	long := strings.Repeat("word ", 20000)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("note %d", i+1)
	}
	snap := model.NewSnapshot([]model.Issue{{ID: "n-1", Title: "T", Status: "open", Notes: strings.Join(lines, "\n\n"), Description: long}}, model.Readiness{}, now)
	p := New()
	in := input(t, snap, "n-1", Overlay, 100, 120)
	p.Render(in)
	p.Next()
	p.Next()
	p.Expand()
	out := text(p.Render(in))
	if !strings.Contains(out, "note 40") {
		t.Errorf("notes must not be capped:\n%s", out)
	}
	for _, j := range p.Plan(in) {
		if len(j.text) > maxMarkdownBytes+len("…") {
			t.Errorf("job carries %d bytes", len(j.text))
		}
	}
}

func toSection(p *Panel, s Section) {
	for range int(sectionCount) {
		if p.Cursor() == s {
			return
		}
		p.Next()
	}
}

func TestIssueRowsTakeTheCursor(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1", Overlay, 100, 60)
	in.Focused = true
	p.Render(in)
	toSection(p, Children)
	if p.Cursor() != Children {
		t.Fatalf("cursor %v", p.Cursor())
	}
	if _, ok := p.Row(); ok {
		t.Fatal("the cursor starts on the heading")
	}
	if !p.Move(1) {
		t.Fatal("j steps from the heading to the first row")
	}
	p.Render(in)
	if id, ok := p.Row(); !ok || id != "d-1.2" {
		t.Fatalf("row %q %v", id, ok)
	}
	if !p.Move(1) || !p.OnFold() {
		t.Fatal("the closed-children row is the next stop")
	}
	if p.Move(1) {
		t.Error("no stop after the last row")
	}
	for range 2 {
		if !p.Move(-1) {
			t.Fatal("k walks back to the heading")
		}
	}
	if p.Move(-1) {
		t.Error("k on the heading scrolls instead")
	}
}

func TestFocusedRowIsMarked(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1", Overlay, 100, 60)
	in.Focused = true
	p.Render(in)
	toSection(p, Children)
	p.Move(1)
	marked := ""
	for _, l := range strings.Split(text(p.Render(in)), "\n") {
		if strings.Contains(l, "d-1.2") && strings.HasPrefix(l, "|") {
			marked = l
		}
	}
	if marked == "" {
		t.Errorf("focused row carries no band:\n%s", text(p.Render(in)))
	}
}

func TestDependencyRowsAreStops(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1.2", Overlay, 100, 60)
	p.Render(in)
	toSection(p, Dependencies)
	var ids []string
	for p.Move(1) {
		p.Render(in)
		id, _ := p.Row()
		ids = append(ids, id)
	}
	if strings.Join(ids, ",") != "d-1,d-2" {
		t.Errorf("stops %v, want the parent and the blocker", ids)
	}
}

func TestClosedChildrenRowExpandsAndCollapses(t *testing.T) {
	p := New()
	in := input(t, fixture(), "d-1", Overlay, 100, 60)
	in.Focused = true
	p.Render(in)
	toSection(p, Children)
	p.Move(1)
	p.Move(1)
	p.Render(in)
	if strings.Contains(text(p.Render(in)), "d-1.1") {
		t.Fatal("closed children start hidden")
	}
	p.Enter()
	if !strings.Contains(text(p.Render(in)), "d-1.1 First child") {
		t.Fatalf("Enter on the row lists the closed children:\n%s", text(p.Render(in)))
	}
	p.Move(1)
	p.Render(in)
	if id, ok := p.Row(); !ok || id != "d-1.1" {
		t.Fatalf("row %q %v", id, ok)
	}
	p.Collapse()
	out := text(p.Render(in))
	if strings.Contains(out, "d-1.1 First child") || !p.OnFold() || !p.Open(Children) {
		t.Errorf("h among closed children folds them and keeps the section:\n%s", out)
	}
}
