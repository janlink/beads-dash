package config_test

import (
	"errors"
	"testing"

	"github.com/janlink/beads-dash/internal/config"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolvePaths(t *testing.T) {
	tests := []struct {
		name string
		goos string
		home string
		env  map[string]string
		want config.Paths
	}{
		{
			name: "linux defaults",
			goos: "linux", home: "/home/jan",
			want: config.Paths{
				ConfigDir: "/home/jan/.config/bdash", StateDir: "/home/jan/.local/state/bdash",
				ConfigFile: "/home/jan/.config/bdash/config.toml", HistoryFile: "/home/jan/.local/state/bdash/history",
				WorkspacesFile: "/home/jan/.local/state/bdash/workspaces.json",
			},
		},
		{
			name: "darwin uses XDG paths too",
			goos: "darwin", home: "/Users/jan",
			want: config.Paths{
				ConfigDir: "/Users/jan/.config/bdash", StateDir: "/Users/jan/.local/state/bdash",
				ConfigFile: "/Users/jan/.config/bdash/config.toml", HistoryFile: "/Users/jan/.local/state/bdash/history",
				WorkspacesFile: "/Users/jan/.local/state/bdash/workspaces.json",
			},
		},
		{
			name: "XDG variables",
			goos: "linux", home: "/home/jan",
			env: map[string]string{"XDG_CONFIG_HOME": "/xdg/c", "XDG_STATE_HOME": "/xdg/s"},
			want: config.Paths{
				ConfigDir: "/xdg/c/bdash", StateDir: "/xdg/s/bdash",
				ConfigFile: "/xdg/c/bdash/config.toml", HistoryFile: "/xdg/s/bdash/history",
				WorkspacesFile: "/xdg/s/bdash/workspaces.json",
			},
		},
		{
			name: "relative XDG value is ignored",
			goos: "linux", home: "/home/jan",
			env: map[string]string{"XDG_CONFIG_HOME": "rel", "XDG_STATE_HOME": "rel"},
			want: config.Paths{
				ConfigDir: "/home/jan/.config/bdash", StateDir: "/home/jan/.local/state/bdash",
				ConfigFile: "/home/jan/.config/bdash/config.toml", HistoryFile: "/home/jan/.local/state/bdash/history",
				WorkspacesFile: "/home/jan/.local/state/bdash/workspaces.json",
			},
		},
		{
			name: "BDASH_CONFIG_DIR moves config only",
			goos: "linux", home: "/home/jan",
			env: map[string]string{"BDASH_CONFIG_DIR": "/etc/bd"},
			want: config.Paths{
				ConfigDir: "/etc/bd", StateDir: "/home/jan/.local/state/bdash",
				ConfigFile: "/etc/bd/config.toml", HistoryFile: "/home/jan/.local/state/bdash/history",
				WorkspacesFile: "/home/jan/.local/state/bdash/workspaces.json",
			},
		},
		{
			name: "windows AppData",
			goos: "windows", home: `C:\Users\jan`,
			env: map[string]string{"AppData": `C:\Users\jan\AppData\Roaming`, "LocalAppData": `C:\Users\jan\AppData\Local`},
			want: config.Paths{
				ConfigDir: `C:\Users\jan\AppData\Roaming\bdash`, StateDir: `C:\Users\jan\AppData\Local\bdash`,
				ConfigFile:     `C:\Users\jan\AppData\Roaming\bdash\config.toml`,
				HistoryFile:    `C:\Users\jan\AppData\Local\bdash\history`,
				WorkspacesFile: `C:\Users\jan\AppData\Local\bdash\workspaces.json`,
			},
		},
		{
			name: "windows without AppData variables",
			goos: "windows", home: `C:\Users\jan`,
			want: config.Paths{
				ConfigDir: `C:\Users\jan\AppData\Roaming\bdash`, StateDir: `C:\Users\jan\AppData\Local\bdash`,
				ConfigFile:     `C:\Users\jan\AppData\Roaming\bdash\config.toml`,
				HistoryFile:    `C:\Users\jan\AppData\Local\bdash\history`,
				WorkspacesFile: `C:\Users\jan\AppData\Local\bdash\workspaces.json`,
			},
		},
		{
			name: "windows override",
			goos: "windows", home: `C:\Users\jan`,
			env: map[string]string{"AppData": `C:\A`, "LocalAppData": `C:\L`, "BDASH_CONFIG_DIR": `D:\cfg`},
			want: config.Paths{
				ConfigDir: `D:\cfg`, StateDir: `C:\L\bdash`,
				ConfigFile: `D:\cfg\config.toml`, HistoryFile: `C:\L\bdash\history`,
				WorkspacesFile: `C:\L\bdash\workspaces.json`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ResolvePaths(envOf(tt.env), tt.goos, tt.home)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestResolvePathsWithoutHome(t *testing.T) {
	if _, err := config.ResolvePaths(envOf(nil), "linux", ""); !errors.Is(err, config.ErrNoConfigDir) {
		t.Errorf("err = %v, want ErrNoConfigDir", err)
	}
	if _, err := config.ResolvePaths(envOf(nil), "windows", ""); !errors.Is(err, config.ErrNoConfigDir) {
		t.Errorf("windows err = %v, want ErrNoConfigDir", err)
	}
}
