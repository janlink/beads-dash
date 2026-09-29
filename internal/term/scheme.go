package term

import (
	"io"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
)

// IsColorSchemeEvent reports whether msg is a mode 2031 light/dark report.
// Such a report is not trusted: it names the operating system scheme, not the
// terminal background, so the caller re-queries the background on it.
func IsColorSchemeEvent(msg any) bool {
	switch msg.(type) {
	case uv.DarkColorSchemeEvent, uv.LightColorSchemeEvent:
		return true
	}
	return false
}

// EnableColorSchemeReporting turns mode 2031 on and returns an idempotent reset
// that turns it off again. Callers defer it, including on the crash path.
func EnableColorSchemeReporting(w io.Writer) (reset func()) {
	_ = SetColorSchemeReporting(w, true)
	var once sync.Once
	return func() { once.Do(func() { _ = SetColorSchemeReporting(w, false) }) }
}
