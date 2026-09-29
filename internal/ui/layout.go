package ui

// Terminal size limits and breakpoints.
const (
	MinCols = 60
	MinRows = 16
)

// Breakpoint is a width class of the terminal.
type Breakpoint int

// The width classes, from 60 columns up.
const (
	Narrow  Breakpoint = iota // 60-79
	Regular                   // 80-119
	Roomy                     // 120-199
	Wide                      // 200 and up
)

func (b Breakpoint) String() string {
	return [...]string{"narrow", "regular", "roomy", "wide"}[b]
}

// BreakpointOf classifies a terminal width.
func BreakpointOf(cols int) Breakpoint {
	switch {
	case cols >= 200:
		return Wide
	case cols >= 120:
		return Roomy
	case cols >= 80:
		return Regular
	}
	return Narrow
}
