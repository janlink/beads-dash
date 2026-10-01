// Package detail draws the detail panel: a two-line header over accordion
// sections, docked beside a view or shown full screen as an overlay.
package detail

// Widths that decide where the panel sits.
const (
	// SplitMinCols is the width from which the panel sits beside a list:
	// the list keeps ListMinCols and the panel at least sideMinW.
	SplitMinCols = ListMinCols + sideMinW
	// BoardSplitMinCols is the width from which the panel sits beside the
	// Kanban board: two columns of BoardColumnMin and the widest panel.
	BoardSplitMinCols = 2*BoardColumnMin + sideMaxW + 1
	// ListMinCols is the narrowest a list beside the panel may get.
	ListMinCols = 70
	// BoardColumnMin is the narrowest a Kanban column beside the panel may get.
	BoardColumnMin = 24

	// The side panel's width includes its border column.
	sideMinW = 37
	sideMaxW = 41
)

// Frame is how the panel sits on the screen.
type Frame int

// The frames. Hidden means the panel is not shown.
const (
	Hidden Frame = iota
	Side
	Overlay
)

func (f Frame) String() string {
	return [...]string{"hidden", "side", "overlay"}[f]
}

// Dock is the space the panel takes.
type Dock struct {
	Frame Frame
	// W and H are the size of the panel; a side panel spans the full body
	// height.
	W, H int
}

// Place decides where the shown panel sits for a terminal of cols columns and
// a body of body rows: beside the view when the view keeps room for its list
// or, on the board, for two columns; otherwise over it.
func Place(cols, body int, board bool) Dock {
	switch {
	case board && cols >= BoardSplitMinCols:
		return Dock{Frame: Side, W: sideMaxW, H: body}
	case !board && cols >= SplitMinCols:
		return Dock{Frame: Side, W: max(sideMinW, min(sideMaxW, cols-ListMinCols)), H: body}
	}
	return Dock{Frame: Overlay, W: cols, H: body}
}
