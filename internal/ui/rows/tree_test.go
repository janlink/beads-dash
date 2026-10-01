package rows_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func treeRenderer(tier theme.Tier) (*rows.Renderer, []model.TreeRow) {
	snap, st := uitest.Tree()
	r := rows.New(uitest.Look(theme.DepthNone, tier, true))
	r.Bind(snap, st)
	return r, model.BuildTree(snap, st, model.ParseScope("", false).Apply(snap, st), &model.Folds{})
}

func TestTreeRowsHaveExactWidth(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		r, tree := treeRenderer(tier)
		for _, w := range []int{30, 57, 60, 80, 120} {
			for _, row := range tree {
				for _, sel := range []bool{false, true} {
					if got := ansi.StringWidth(r.Tree(row)(w, sel)); got != w {
						t.Errorf("tier %v w %d %s: %d cells", tier, w, row.ID, got)
					}
				}
			}
		}
	}
}

func TestTreeRowFacts(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	r.SetTreeRows(tree)
	find := func(id string, kind model.TreeRowKind) model.TreeRow {
		for _, row := range tree {
			if row.ID == id && row.Kind == kind {
				return row
			}
		}
		t.Fatalf("no row %s", id)
		return model.TreeRow{}
	}
	text := func(row model.TreeRow, w int) string { return ansi.Strip(r.Tree(row)(w, false)) }

	if s := text(find("ws-4k2", model.TreeIssue), 100); !strings.Contains(s, "2/5") || !strings.Contains(s, "#") {
		t.Errorf("parent lacks progress: %q", s)
	}
	if s := text(find("ws-4k2", model.TreeIssue), 60); !strings.Contains(s, "2/5") || strings.Contains(s, "#") {
		t.Errorf("narrow parent keeps the number only: %q", s)
	}
	if s := text(find("ws-4k2.5.1", model.TreeIssue), 100); !strings.Contains(s, "P3") || !strings.Contains(s, "carol") {
		t.Errorf("leaf lacks priority and assignee: %q", s)
	}
	if s := text(find("ws-4k2.5.1", model.TreeIssue), 60); strings.Contains(s, "carol") || !strings.Contains(s, "P3") {
		t.Errorf("narrow leaf drops the assignee: %q", s)
	}
	if s := text(find("ws-lost", model.TreeIssue), 100); !strings.Contains(s, "^ ws-gone") {
		t.Errorf("orphan lacks its marker: %q", s)
	}
	if s := text(find("ws-4k2", model.TreeClosedFold), 100); !strings.Contains(s, "2 closed") {
		t.Errorf("closed row: %q", s)
	}
}

func TestDeepRowsCutTheirStemsFromTheFrontAndKeepTheTitle(t *testing.T) {
	snap, st := uitest.Tree()
	r := rows.New(uitest.Look(theme.DepthNone, theme.TierASCII, true))
	r.Bind(snap, st)
	row := model.TreeRow{ID: "ws-4k2.5.1", Depth: 30, Last: true, More: 1<<30 - 1}
	r.SetTreeRows([]model.TreeRow{row})
	s := ansi.Strip(r.Tree(row)(60, false))
	if !strings.Contains(s, "`- ws-4k2.5.1") || !strings.Contains(s, "...") {
		t.Errorf("own connector, full ID or cut marker missing: %q", s)
	}
	if !strings.Contains(s, "Pickup point lookup") {
		t.Errorf("title lost: %q", s)
	}
	if got := ansi.StringWidth(s); got != 60 {
		t.Errorf("row is %d cells", got)
	}
}

func TestGlyphColumnHoldsCaretOrStatusAndNeverIndents(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	r.SetTreeRows(tree)
	g := theme.GlyphsFor(theme.TierASCII)
	for _, row := range tree {
		s := ansi.Strip(r.Tree(row)(120, false))
		got := s[:1]
		isCaret := got == g.FoldClosed || got == g.FoldOpen
		switch {
		case row.Foldable && row.Folded && got != g.FoldClosed,
			row.Foldable && !row.Folded && got != g.FoldOpen,
			!row.Foldable && (isCaret || got == " "),
			s[1] != ' ':
			t.Errorf("%s %v: glyph column %q in %q", row.ID, row.Kind, got, s)
		}
	}
}

func TestClosedRowShowsStemsAndCountInTheIDColumn(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	r.SetTreeRows(tree)
	found := false
	for _, row := range tree {
		if row.Kind != model.TreeClosedFold {
			continue
		}
		found = true
		s := ansi.Strip(r.Tree(row)(100, false))
		if !strings.HasPrefix(s, "> `- x 2 closed") {
			t.Errorf("closed row = %q", s)
		}
	}
	if !found {
		t.Fatal("the sample tree has no closed-children row")
	}
}

func TestReadyColumnsDropInOrder(t *testing.T) {
	r, _ := treeRenderer(theme.TierASCII)
	for _, tc := range []struct {
		w    int
		want rows.ReadyCols
	}{
		{200, rows.ReadyCols{Type: true, Assignee: true, Age: true, Reason: true}},
		{120, rows.ReadyCols{Type: true, Assignee: true, Age: true, Reason: true}},
		{119, rows.ReadyCols{Type: true, Assignee: true, Age: true}},
		{99, rows.ReadyCols{Type: true, Assignee: true}},
		{89, rows.ReadyCols{Type: true, Assignee: true}},
		{79, rows.ReadyCols{Type: true}},
		{60, rows.ReadyCols{Type: true}},
	} {
		if got := r.ReadyColumns(tc.w); got != tc.want {
			t.Errorf("w %d: %+v, want %+v", tc.w, got, tc.want)
		}
	}
}

func TestReadyRowsHaveExactWidthAndTheirColumns(t *testing.T) {
	r, _ := treeRenderer(theme.TierASCII)
	now := uitest.T0
	for _, w := range []int{60, 80, 100, 120, 200} {
		cols := r.ReadyColumns(w)
		for _, pinned := range []bool{false, true} {
			s := r.Ready(rows.ReadyRow{ID: "ws-lost", Cols: cols, Now: now, Reason: "waits on ws-1", Pinned: pinned})(w-rows.GutterWidth, false)
			if got := ansi.StringWidth(s); got != w-rows.GutterWidth {
				t.Errorf("w %d: %d cells", w, got)
			}
		}
	}
	s := ansi.Strip(r.Ready(rows.ReadyRow{ID: "ws-lost", Cols: r.ReadyColumns(200), Now: now, Reason: "no blockers"})(197, false))
	for _, want := range []string{"ws-lost", "P2", "task", "alice", "1d", "no blockers"} {
		if !strings.Contains(s, want) {
			t.Errorf("row lacks %q: %q", want, s)
		}
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{2 * 24 * time.Hour, "2d"},
		{15 * 24 * time.Hour, "2w"},
		{100 * 24 * time.Hour, "3mo"},
		{800 * 24 * time.Hour, "2y"},
	} {
		if got := rows.Age(now, now.Add(-tc.ago)); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.ago, got, tc.want)
		}
	}
	if rows.Age(now, time.Time{}) != "" || rows.Age(now, now.Add(time.Hour)) != "now" {
		t.Error("zero and future times")
	}
}

func TestGutterBarFollowsThePriority(t *testing.T) {
	snap, st := uitest.Sample()
	p := theme.NewPalette(theme.Default(), true, theme.DepthTrueColor)
	r := rows.New(uitest.Look(theme.DepthTrueColor, theme.TierFancy, true))
	r.Bind(snap, st)
	for _, tc := range []struct {
		id   string
		role theme.Role
	}{
		{"ws-7mt", theme.Priority2},
		{"ws-5ca", theme.Rule},
		{"ws-9qe", theme.Priority0},
	} {
		if g, want := r.Gutter(rows.Row{ID: tc.id}), p.Style(tc.role).Render("▌"); !strings.HasPrefix(g, want) {
			t.Errorf("%s: gutter %q does not start with the %s bar %q", tc.id, g, tc.role, want)
		}
		if !strings.Contains(ansi.Strip(r.Gutter(rows.Row{ID: tc.id, Current: true})), "▌") {
			t.Errorf("%s: selected gutter lost its bar", tc.id)
		}
	}
	if g, want := r.Gutter(rows.Row{ID: "ws-7mt", Closed: true}), p.Style(theme.Rule).Render("▌"); !strings.HasPrefix(g, want) {
		t.Errorf("a Closed row does not draw the closed bar: %q", g)
	}
	if g := r.Gutter(rows.Row{}); strings.Contains(g, "▌") {
		t.Errorf("a row without an issue draws a bar: %q", g)
	}
}

func TestNoBarWithoutColourOrUnicode(t *testing.T) {
	snap, st := uitest.Sample()
	for _, l := range []struct {
		depth theme.Depth
		tier  theme.Tier
	}{{theme.DepthNone, theme.TierFancy}, {theme.DepthTrueColor, theme.TierASCII}} {
		r := rows.New(uitest.Look(l.depth, l.tier, true))
		r.Bind(snap, st)
		if g := ansi.Strip(r.Gutter(rows.Row{ID: "ws-7mt"})); strings.TrimSpace(g) != "" {
			t.Errorf("%v/%v: idle gutter %q", l.depth, l.tier, g)
		}
	}
}

func TestTreeColumnsLineUpAcrossDepths(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	r.SetTreeRows(tree)
	titleAt := map[string]int{}
	for _, row := range tree {
		if row.Kind != model.TreeIssue {
			continue
		}
		is, _ := uitestTreeIssue(row.ID)
		s := ansi.Strip(r.Tree(row)(120, false))
		titleAt[row.ID] = strings.Index(s, is)
	}
	first := -1
	for id, at := range titleAt {
		if first < 0 {
			first = at
		}
		if at < 0 || at != first {
			t.Errorf("%s title starts at %d, others at %d", id, at, first)
		}
	}
}

func uitestTreeIssue(id string) (string, bool) {
	snap, _ := uitest.Tree()
	is, ok := snap.Issue(id)
	if !ok {
		return "", false
	}
	return is.Title, true
}

func TestTypeColumnIsReservedBeforeTheTitle(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	for _, row := range tree {
		if row.Kind != model.TreeIssue || row.ID != "ws-4k2.5.1" {
			continue
		}
		r.SetTreeRows(tree)
		if s := ansi.Strip(r.Tree(row)(100, false)); !strings.Contains(s, " task ") {
			t.Errorf("type word missing at 100: %q", s)
		}
		if s := ansi.Strip(r.Tree(row)(40, false)); strings.Contains(s, " task ") {
			t.Errorf("type word kept at 40 over the title: %q", s)
		}
	}
}

func TestNoBarsWithoutDistinctColours(t *testing.T) {
	snap, st := uitest.Sample()
	mono, _ := theme.Lookup("monochrome")
	l := look.New(theme.NewPalette(mono, true, theme.Depth16), theme.GlyphsFor(theme.TierFancy))
	r := rows.New(l)
	r.Bind(snap, st)
	if g := ansi.Strip(r.Gutter(rows.Row{ID: "ws-7mt"})); strings.TrimSpace(g) != "" {
		t.Errorf("colourless gutter draws a bar: %q", g)
	}
	if g := ansi.Strip(r.Gutter(rows.Row{ID: "ws-7mt", Current: true})); !strings.HasPrefix(g, "▌") {
		t.Errorf("current band lost: %q", g)
	}
}

func TestPinnedReadyRowsKeepTheirColumns(t *testing.T) {
	r, _ := treeRenderer(theme.TierASCII)
	now := uitest.T0
	for _, w := range []int{80, 100, 119} {
		cols := r.ReadyColumns(w)
		at := func(pinned bool) int {
			s := ansi.Strip(r.Ready(rows.ReadyRow{ID: "ws-4k2.5.1", Cols: cols, Now: now, Reason: "waits on ws-1", Pinned: pinned})(w-rows.GutterWidth, false))
			return strings.Index(s, "P3")
		}
		if a, b := at(false), at(true); a != b {
			t.Errorf("w %d: priority at %d pinned, %d otherwise", w, b, a)
		}
	}
}

func TestTreeAlignsAcrossDepthsAndWidths(t *testing.T) {
	snap, st := uitest.Tree()
	for _, tier := range []theme.Tier{theme.TierASCII, theme.TierFancy} {
		r := rows.New(uitest.Look(theme.DepthNone, tier, true))
		r.Bind(snap, st)
		for _, depth := range []int{0, 1, 3, 8, 20} {
			var tree []model.TreeRow
			for d := 0; d <= depth; d++ {
				id := "ws-4k2.5.1"
				if d%2 == 1 {
					id = "ws-9qe"
				}
				tree = append(tree, model.TreeRow{Kind: model.TreeIssue, ID: id, Depth: d, Last: d%3 == 0, More: 1<<uint(min(d, 62)) - 1})
			}
			if depth > 0 {
				tree = append(tree, model.TreeRow{Kind: model.TreeClosedFold, ID: "ws-4k2", Depth: depth, Foldable: true, Folded: true, Closed: 3, Last: true})
			}
			r.SetTreeRows(tree)
			for _, width := range []int{60, 80, 120, 200} {
				w := width - rows.GutterWidth
				header := ansi.Strip(r.TreeHeader(width))[rows.GutterWidth:]
				want := map[string]int{"TITLE": strings.Index(header, "TITLE"), "PR": strings.Index(header, "PR")}
				if got := ansi.StringWidth(header); got != w {
					t.Errorf("%v d%d w%d: header is %d cells", tier, depth, width, got)
				}
				for _, row := range tree {
					s := ansi.Strip(r.Tree(row)(w, false))
					if got := ansi.StringWidth(s); got != w {
						t.Errorf("%v d%d w%d %s: %d cells", tier, depth, width, row.ID, got)
					}
					if row.Kind == model.TreeClosedFold {
						continue
					}
					title := strings.Index(s, "Pickup")
					if title < 0 {
						title = strings.Index(s, "Payment")
					}
					if title < 0 || ansi.StringWidth(s[:title]) != want["TITLE"] {
						t.Errorf("%v d%d w%d %s: title column off header (%d): %q", tier, depth, width, row.ID, want["TITLE"], s)
					}
					if p := strings.LastIndex(s, " P"); p < 0 || ansi.StringWidth(s[:p+1]) != want["PR"] {
						t.Errorf("%v d%d w%d %s: PR column off header (%d): %q", tier, depth, width, row.ID, want["PR"], s)
					}
				}
			}
		}
	}
}

func TestParentStatusSurvivesWithoutColour(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	r.SetTreeRows(tree)
	for _, row := range tree {
		if row.ID != "ws-4k2" || row.Kind != model.TreeIssue {
			continue
		}
		if s := ansi.Strip(r.Tree(row)(100, false)); !strings.Contains(s, "* 2/5") {
			t.Errorf("parent lacks its status glyph before the count: %q", s)
		}
	}
}
