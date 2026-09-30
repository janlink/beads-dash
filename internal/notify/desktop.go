package notify

import (
	"github.com/janlink/beads-dash/internal/host"
)

// desktopBackend is one way to raise a desktop notification.
type desktopBackend struct {
	name string
	argv func(Message) []string
}

// selectDesktop picks the desktop backend of env. Windows and WSL have none.
func selectDesktop(env host.Env, look func(string) (string, error)) desktopBackend {
	if env.GOOS == "windows" || env.WSL {
		return desktopBackend{}
	}
	found := func(name string) (string, bool) {
		p, err := look(name)
		return p, err == nil
	}
	switch env.GOOS {
	case "darwin":
		if p, ok := found("osascript"); ok {
			return osascript(p)
		}
	case "linux":
		if p, ok := found("notify-send"); ok {
			return desktopBackend{
				name: "notify-send",
				argv: func(m Message) []string { return []string{p, "--app-name=bdash", "--", m.Title, m.Body} },
			}
		}
	}
	return desktopBackend{}
}

// osascript passes title and body as script arguments, so nothing needs
// quoting for AppleScript.
func osascript(path string) desktopBackend {
	return desktopBackend{
		name: "osascript",
		argv: func(m Message) []string {
			return []string{
				path,
				"-e", "on run argv",
				"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
				"-e", "end run",
				m.Title, m.Body,
			}
		},
	}
}
