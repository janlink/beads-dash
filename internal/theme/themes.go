package theme

import "strings"

// Theme is a named set of role colours. Dark and Light are authored in hex;
// ANSI16 is the one hand-authored table for 16-colour terminals, which has no
// light/dark split because the terminal theme remaps those colours.
//
// The Surface token of each hex table is the background the theme was authored
// against; contrast is measured on it.
type Theme struct {
	Name   string
	Dark   [roleCount]string
	Light  [roleCount]string
	ANSI16 [roleCount]Attr
}

// Attr is a colourless-capable text attribute set. FG is an ANSI index 0..15,
// or DefaultFG for the terminal's default foreground.
type Attr struct {
	FG        int
	Bold      bool
	Faint     bool
	Underline bool
	Reverse   bool
}

// DefaultFG selects the terminal's default foreground in an Attr.
const DefaultFG = -1

// Hex returns the authored hex colour of a role for one variant.
func (t Theme) Hex(dark bool, r Role) string {
	if dark {
		return t.Dark[r]
	}
	return t.Light[r]
}

// DefaultName is the theme used when none is chosen.
const DefaultName = "default"

// Names lists the shipped theme names in display order.
func Names() []string {
	out := make([]string, len(themes))
	for i, t := range themes {
		out[i] = t.Name
	}
	return out
}

// Lookup finds a shipped theme by case-insensitive name.
func Lookup(name string) (Theme, bool) {
	for _, t := range themes {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Theme{}, false
}

// Default returns the default theme.
func Default() Theme { return themes[0] }

// spec is the authoring form of one hex variant; expand fills every role.
type spec struct {
	surface, selection              string
	strong, text, dim, faint, rule  string
	border                          string
	primary, success, errc, warning string
	yellow, cyan, violet            string
	changed, match                  string
}

func (s spec) expand() (t [roleCount]string) {
	t[StatusOpen] = s.primary
	t[StatusInProgress] = s.yellow
	t[StatusBlocked] = s.errc
	t[StatusFrozen] = s.cyan
	t[StatusClosed] = s.faint
	t[StatusOther] = s.violet
	t[Priority0] = s.errc
	t[Priority1] = s.warning
	t[Priority2] = s.yellow
	t[Priority3] = s.primary
	t[Priority4] = s.faint
	t[TypeBug] = s.errc
	t[TypeFeature] = s.success
	t[TypeTask] = s.text
	t[TypeEpic] = s.violet
	t[TypeChore] = s.dim
	t[TypeDecision] = s.cyan
	t[TypeOther] = s.dim
	t[Strong] = s.strong
	t[Text] = s.text
	t[Dim] = s.dim
	t[Faint] = s.faint
	t[Rule] = s.rule
	t[Primary] = s.primary
	t[Border] = s.border
	t[Surface] = s.surface
	t[Success] = s.success
	t[Error] = s.errc
	t[Warning] = s.warning
	t[Selection] = s.selection
	t[Changed] = s.changed
	t[Match] = s.match
	return t
}

func mono(fg int, a Attr) Attr {
	a.FG = fg
	return a
}

// ansi16 builds a theme's 16-colour table. The text ladder uses the default
// foreground with bold/faint, never white.
func ansi16(primary int, colourless bool) (t [roleCount]Attr) {
	c := func(fg int, a Attr) Attr {
		if colourless {
			fg = DefaultFG
		}
		return mono(fg, a)
	}
	t[Strong] = Attr{FG: DefaultFG, Bold: true}
	t[Text] = Attr{FG: DefaultFG}
	t[Dim] = Attr{FG: DefaultFG, Faint: true}
	t[Faint] = c(8, Attr{Faint: true})
	t[Rule] = c(8, Attr{Faint: true})

	t[StatusOpen] = c(primary, Attr{})
	t[StatusInProgress] = c(3, Attr{Bold: colourless})
	t[StatusBlocked] = c(1, Attr{Bold: true})
	t[StatusFrozen] = c(6, Attr{Faint: colourless})
	t[StatusClosed] = c(8, Attr{Faint: true})
	t[StatusOther] = c(5, Attr{Faint: colourless})

	t[Priority0] = c(1, Attr{Bold: true})
	t[Priority1] = c(3, Attr{Bold: true})
	t[Priority2] = c(3, Attr{})
	t[Priority3] = c(primary, Attr{})
	t[Priority4] = c(8, Attr{Faint: true})

	t[TypeBug] = c(1, Attr{})
	t[TypeFeature] = c(2, Attr{})
	t[TypeTask] = Attr{FG: DefaultFG}
	t[TypeEpic] = c(5, Attr{Bold: colourless})
	t[TypeChore] = Attr{FG: DefaultFG, Faint: true}
	t[TypeDecision] = c(6, Attr{Underline: colourless})
	t[TypeOther] = Attr{FG: DefaultFG, Faint: true}

	t[Primary] = c(primary, Attr{Bold: true})
	t[Border] = c(8, Attr{})
	t[Surface] = Attr{FG: DefaultFG}
	t[Success] = c(2, Attr{})
	t[Error] = c(1, Attr{Bold: true})
	t[Warning] = c(3, Attr{})
	t[Selection] = Attr{FG: DefaultFG, Reverse: true}
	t[Changed] = c(11, Attr{Bold: true})
	t[Match] = c(6, Attr{Underline: true, Bold: colourless})
	return t
}

var themes = []Theme{
	{
		Name: "default",
		Dark: spec{
			surface: "#1e1e2e", selection: "#313244",
			strong: "#f5f5f7", text: "#cdd6f4", dim: "#a6adc8", faint: "#8087a2", rule: "#45475a",
			border:  "#585b70",
			primary: "#89b4fa", success: "#a6e3a1", errc: "#f38ba8", warning: "#fab387",
			yellow: "#f9e2af", cyan: "#74c7ec", violet: "#b4befe",
			changed: "#f9e2af", match: "#94e2d5",
		}.expand(),
		Light: spec{
			surface: "#eff1f5", selection: "#ccd0da",
			strong: "#11111b", text: "#4c4f69", dim: "#50536b", faint: "#63667c", rule: "#bcc0cc",
			border:  "#9ca0b0",
			primary: "#1e66f5", success: "#2d7a1c", errc: "#c0173d", warning: "#b34d0a",
			yellow: "#8a5a00", cyan: "#0f6f8f", violet: "#5b4fd1",
			changed: "#8a5a00", match: "#0d7a6a",
		}.expand(),
		ANSI16: ansi16(12, false),
	},
	{
		Name: "ocean",
		Dark: spec{
			surface: "#0b1e2d", selection: "#16374f",
			strong: "#f0f8ff", text: "#cfe3f1", dim: "#9dbcd3", faint: "#7095ad", rule: "#26465c",
			border:  "#3d6580",
			primary: "#4fc3f7", success: "#6fd6a0", errc: "#ff7b8a", warning: "#ffb454",
			yellow: "#ffe08a", cyan: "#5eead4", violet: "#a5b4fc",
			changed: "#ffe08a", match: "#5eead4",
		}.expand(),
		Light: spec{
			surface: "#f2f8fc", selection: "#cfe3f1",
			strong: "#06202f", text: "#16384d", dim: "#35586f", faint: "#4d7288", rule: "#b7cfde",
			border:  "#8fb0c5",
			primary: "#0369a1", success: "#157347", errc: "#b42318", warning: "#a84b08",
			yellow: "#7f5a00", cyan: "#0e7490", violet: "#4f46e5",
			changed: "#7f5a00", match: "#0a7c72",
		}.expand(),
		ANSI16: ansi16(14, false),
	},
	{
		Name: "forest",
		Dark: spec{
			surface: "#14201a", selection: "#24382d",
			strong: "#f2fbf4", text: "#d4e6d8", dim: "#a3bfa9", faint: "#7a9a82", rule: "#34493c",
			border:  "#4d6b57",
			primary: "#7fd18b", success: "#a3e635", errc: "#f47174", warning: "#f2a65a",
			yellow: "#ecd06f", cyan: "#63d6c0", violet: "#b7a5e8",
			changed: "#ecd06f", match: "#63d6c0",
		}.expand(),
		Light: spec{
			surface: "#f4f8f1", selection: "#d3e3d0",
			strong: "#0f1f14", text: "#23402b", dim: "#40604a", faint: "#587a62", rule: "#bcd1bb",
			border:  "#92b099",
			primary: "#1f7a3a", success: "#3f7d0c", errc: "#b3261e", warning: "#a4530a",
			yellow: "#7c5f00", cyan: "#0f766e", violet: "#5b4fb3",
			changed: "#7c5f00", match: "#0f766e",
		}.expand(),
		ANSI16: ansi16(10, false),
	},
	{
		Name: "sunset",
		Dark: spec{
			surface: "#24151f", selection: "#45283a",
			strong: "#fff4ee", text: "#f1d9d2", dim: "#cfa9a4", faint: "#a98a8b", rule: "#573a4b",
			border:  "#7a566a",
			primary: "#ff9e64", success: "#b9d97a", errc: "#ff6b81", warning: "#ffb86b",
			yellow: "#ffd97a", cyan: "#7fd6d1", violet: "#d6a5f5",
			changed: "#ffd97a", match: "#7fd6d1",
		}.expand(),
		Light: spec{
			surface: "#fdf4ee", selection: "#f5d9cb",
			strong: "#2b120d", text: "#4a2a22", dim: "#6b443a", faint: "#82594e", rule: "#e6c8b8",
			border:  "#c9a08e",
			primary: "#b8470b", success: "#4d7c0f", errc: "#b91c3c", warning: "#9a4a08",
			yellow: "#7a4a08", cyan: "#0e7490", violet: "#7e3aa8",
			changed: "#7a4a08", match: "#0e7490",
		}.expand(),
		ANSI16: ansi16(9, false),
	},
	{
		Name: "monochrome",
		Dark: spec{
			surface: "#121212", selection: "#303030",
			strong: "#ffffff", text: "#d0d0d0", dim: "#a8a8a8", faint: "#878787", rule: "#3a3a3a",
			border:  "#5a5a5a",
			primary: "#e8e8e8", success: "#bcbcbc", errc: "#f5f5f5", warning: "#d8d8d8",
			yellow: "#c4c4c4", cyan: "#b0b0b0", violet: "#9c9c9c",
			changed: "#ffffff", match: "#ffffff",
		}.expand(),
		Light: spec{
			surface: "#f5f5f5", selection: "#d6d6d6",
			strong: "#000000", text: "#262626", dim: "#4a4a4a", faint: "#5e5e5e", rule: "#c4c4c4",
			border:  "#9a9a9a",
			primary: "#1a1a1a", success: "#3a3a3a", errc: "#000000", warning: "#2a2a2a",
			yellow: "#4a4a4a", cyan: "#555555", violet: "#606060",
			changed: "#000000", match: "#000000",
		}.expand(),
		ANSI16: ansi16(DefaultFG, true),
	},
}
