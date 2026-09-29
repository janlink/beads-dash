package rows

import (
	"strconv"
	"time"
)

// Age is the short age of t as of now: now, 5m, 3h, 2d, 6w, 4mo, 2y. A zero t
// has no age.
func Age(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := max(now.Sub(t), 0)
	n := func(v time.Duration, unit string) string { return strconv.Itoa(int(v)) + unit }
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return n(d/time.Minute, "m")
	case d < 24*time.Hour:
		return n(d/time.Hour, "h")
	case d < 7*24*time.Hour:
		return n(d/(24*time.Hour), "d")
	case d < 30*24*time.Hour:
		return n(d/(7*24*time.Hour), "w")
	case d < 365*24*time.Hour:
		return n(d/(30*24*time.Hour), "mo")
	}
	return n(d/(365*24*time.Hour), "y")
}
