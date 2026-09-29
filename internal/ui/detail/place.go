// Package detail draws the detail panel: a two-line header over accordion
// sections, docked beside or below a view or shown full screen as an overlay.
package detail

// Terminal sizes that decide where the panel sits.
const (
	// SideMinCols is the width from which the panel docks at the right.
	SideMinCols = 200
	// BottomMinCols is the width from which the panel docks at the bottom.
	BottomMinCols = 80
	// MinBody is the body height any docked panel needs.
	MinBody = 20

	sideShare   = 45
	sideMinW    = 60
	bottomShare = 40
	bottomMinH  = 10
)

// Frame is how the panel sits on the screen.
type Frame int

// The frames. Hidden means nothing is docked, so the detail opens as an
// overlay that takes the keys.
const (
	Hidden Frame = iota
	Side
	Bottom
	Overlay
)

func (f Frame) String() string {
	return [...]string{"hidden", "side", "bottom", "overlay"}[f]
}

// Dock is the space the docked panel takes.
type Dock struct {
	Frame Frame
	// W and H are the size of the panel; a bottom panel spans the full
	// width and a side panel the full body height.
	W, H int
}

// Place decides where the docked panel sits for a terminal of cols columns
// and a body of body rows. docked is the user's show/hide setting.
func Place(cols, body int, docked bool) Dock {
	switch {
	case !docked:
	case cols >= SideMinCols && body >= MinBody:
		return Dock{Frame: Side, W: max(cols*sideShare/100, sideMinW), H: body}
	case cols >= BottomMinCols && body >= MinBody:
		return Dock{Frame: Bottom, W: cols, H: max(body*bottomShare/100, bottomMinH)}
	}
	return Dock{Frame: Hidden}
}
