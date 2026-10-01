package theme

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/width"
)

// Tier is a glyph tier: the character set bdash draws with.
type Tier int

const (
	TierAuto Tier = iota
	TierFancy
	TierSafe
	TierASCII
)

var tierNames = [...]string{"auto", "fancy", "safe", "ascii"}

func (t Tier) String() string {
	if t < 0 || int(t) >= len(tierNames) {
		return "unknown"
	}
	return tierNames[t]
}

// ParseTier parses auto|fancy|safe|ascii.
func ParseTier(s string) (Tier, bool) {
	for i, n := range tierNames {
		if s == n {
			return Tier(i), true
		}
	}
	return TierAuto, false
}

// ResolveTier turns auto into fancy, or ascii when the locale is clearly not
// UTF-8. A concrete tier is returned unchanged.
func ResolveTier(t Tier, getenv func(string) string) Tier {
	if t != TierAuto {
		return t
	}
	if localeIsNotUTF8(getenv) {
		return TierASCII
	}
	return TierFancy
}

func localeIsNotUTF8(getenv func(string) string) bool {
	var loc string
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := getenv(k); v != "" {
			loc = v
			break
		}
	}
	if loc == "" {
		return false
	}
	if loc == "C" || loc == "POSIX" {
		return true
	}
	_, charset, ok := strings.Cut(loc, ".")
	if !ok {
		return false
	}
	charset, _, _ = strings.Cut(charset, "@")
	charset = strings.ToLower(strings.ReplaceAll(charset, "-", ""))
	return charset != "utf8"
}

// AmbiguousNotice is the footer text shown when the tier fell back to ascii
// because ambiguous-width glyphs report wide.
const AmbiguousNotice = "ambiguous glyphs are wide here: using ascii glyphs (ambiguous = narrow overrides)"

// FallbackForAmbiguous returns ascii and a footer notice when the terminal
// draws ambiguous-width glyphs wide and the tier is not already ascii.
func FallbackForAmbiguous(t Tier, ambiguousWide bool) (Tier, string) {
	if t != TierASCII && ambiguousWide {
		return TierASCII, AmbiguousNotice
	}
	return t, ""
}

// Glyphs is the glyph set of one tier. Members of an interchangeable slot
// group are padded to the widest member of that group.
type Glyphs struct {
	Tier Tier

	// Status is indexed by presentation status: Open, In progress, Blocked,
	// Frozen, Closed, Other.
	Status [6]string

	Band   string
	Mark   string
	Change string

	FoldOpen   string
	FoldClosed string

	Branch     string
	LastBranch string
	Vertical   string

	BarFull  string
	BarEmpty string

	Checked   string
	Unchecked string

	Rule     string
	Ellipsis string
	Arrow    string
	Bullet   string
	// Enter labels the Enter key in hints.
	Enter string
	// Live and Stale mark the snapshot state in the header.
	Live  string
	Stale string
}

// Slot is one named glyph of a set.
type Slot struct {
	Name  string
	Glyph string
}

// Slots lists every glyph of the set with its slot name.
func (g Glyphs) Slots() []Slot {
	out := []Slot{
		{"band", g.Band},
		{"mark", g.Mark},
		{"change", g.Change},
		{"fold_open", g.FoldOpen},
		{"fold_closed", g.FoldClosed},
		{"branch", g.Branch},
		{"last_branch", g.LastBranch},
		{"vertical", g.Vertical},
		{"bar_full", g.BarFull},
		{"bar_empty", g.BarEmpty},
		{"checked", g.Checked},
		{"unchecked", g.Unchecked},
		{"rule", g.Rule},
		{"ellipsis", g.Ellipsis},
		{"arrow", g.Arrow},
		{"bullet", g.Bullet},
		{"enter", g.Enter},
		{"live", g.Live},
		{"stale", g.Stale},
	}
	return append(out, g.statusSlots()...)
}

func (g Glyphs) statusSlots() []Slot {
	out := make([]Slot, len(g.Status))
	for i, s := range g.Status {
		out[i] = Slot{StatusRole(i).String(), s}
	}
	return out
}

// ExclusiveSlots lists the slots that share one cell region in a row and must
// therefore be distinct from each other: statuses, fold markers, band, mark and
// change.
func (g Glyphs) ExclusiveSlots() []Slot {
	out := []Slot{
		{"band", g.Band},
		{"mark", g.Mark},
		{"change", g.Change},
		{"fold_open", g.FoldOpen},
		{"fold_closed", g.FoldClosed},
	}
	return append(out, g.statusSlots()...)
}

// HasAmbiguous reports whether any glyph of the set is in the ambiguous-width
// class, which the width probe decides on.
func (g Glyphs) HasAmbiguous() bool {
	for _, s := range g.Slots() {
		for _, r := range s.Glyph {
			if IsAmbiguous(r) {
				return true
			}
		}
	}
	return false
}

// IsAmbiguous reports whether r has East Asian ambiguous width.
func IsAmbiguous(r rune) bool { return width.LookupRune(r).Kind() == width.EastAsianAmbiguous }

// GlyphsFor returns the glyph set of a concrete tier; auto is treated as
// fancy.
func GlyphsFor(t Tier) Glyphs {
	var g Glyphs
	switch t {
	case TierASCII:
		g = Glyphs{
			Status: [6]string{"o", "*", "!", "~", "x", "?"},
			Band:   "|", Mark: "#", Change: "+",
			FoldOpen: "v", FoldClosed: ">",
			Branch: "|-", LastBranch: "`-", Vertical: "|",
			BarFull: "#", BarEmpty: "-",
			Checked: "[x]", Unchecked: "[ ]",
			Rule: "-", Ellipsis: "...", Arrow: "->", Bullet: "*", Enter: "Enter",
			Live: "*", Stale: "o",
		}
	case TierSafe:
		g = Glyphs{
			Status: [6]string{"○", "◙", "●", "◊", "√", "·"},
			Band:   "▌", Mark: "■", Change: "*",
			FoldOpen: "▼", FoldClosed: "►",
			Branch: "├─", LastBranch: "└─", Vertical: "│",
			BarFull: "█", BarEmpty: "░",
			Checked: "■", Unchecked: "□",
			Rule: "─", Ellipsis: "…", Arrow: "→", Bullet: "•", Enter: "Enter",
			Live: "●", Stale: "○",
		}
	case TierAuto, TierFancy:
		t = TierFancy
		g = Glyphs{
			Status: [6]string{"○", "◐", "●", "▫", "✓", "◊"},
			Band:   "▌", Mark: "■", Change: "✱",
			FoldOpen: "▾", FoldClosed: "▸",
			Branch: "├─", LastBranch: "└─", Vertical: "│",
			BarFull: "█", BarEmpty: "░",
			Checked: "▪", Unchecked: "▫",
			Rule: "─", Ellipsis: "…", Arrow: "→", Bullet: "•", Enter: "↵",
			Live: "●", Stale: "◌",
		}
	}
	g.Tier = t
	status := make([]*string, len(g.Status))
	for i := range g.Status {
		status[i] = &g.Status[i]
	}
	padGroup(status...)
	padGroup(&g.FoldOpen, &g.FoldClosed)
	padGroup(&g.Branch, &g.LastBranch, &g.Vertical)
	padGroup(&g.BarFull, &g.BarEmpty)
	padGroup(&g.Checked, &g.Unchecked)
	return g
}

func padGroup(members ...*string) {
	widest := 0
	for _, p := range members {
		widest = max(widest, ansi.StringWidth(*p))
	}
	for _, p := range members {
		if pad := widest - ansi.StringWidth(*p); pad > 0 {
			*p += strings.Repeat(" ", pad)
		}
	}
}
