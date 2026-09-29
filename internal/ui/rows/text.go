package rows

import "github.com/charmbracelet/x/ansi"

// MidCut shortens s to w cells by cutting out the middle and putting
// ellipsis there, for text that differs at its end, such as agent names.
func MidCut(s string, w int, ellipsis string) string {
	total := ansi.StringWidth(s)
	ew := ansi.StringWidth(ellipsis)
	if total <= w || w <= ew {
		return s
	}
	keep := w - ew
	head := (keep + 1) / 2
	tail := keep - head
	return ansi.Truncate(s, head, "") + ellipsis + ansi.TruncateLeft(s, total-tail, "")
}
