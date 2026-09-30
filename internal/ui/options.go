package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/ui/keys"
)

// Engine is the refresh engine as the shell uses it.
type Engine interface {
	Start(ctx context.Context)
	Stop()
	Updates() <-chan refresh.Update
	Refresh()
	SetFocus(focused bool)
	// EnableEvents switches the engine to events mode once the journal is on.
	EnableEvents()
	// Do runs a lazy bd read in the serialized queue.
	Do(ctx context.Context, fn func(context.Context, bd.Client) error) error
	// Write runs a bd write in the serialized queue. ids are the issues it
	// touches, so their events are credited to the user; the engine refreshes
	// right after, even when fn fails.
	Write(ctx context.Context, ids []string, fn func(context.Context, bd.Client) error) error
}

// Persister writes one setting to the config file.
type Persister interface {
	Set(name string, value any) error
}

// Options is everything the shell needs from startup.
type Options struct {
	Keys *keys.Map
	// Client is the bd boundary; the shell opens the session and hands the
	// client to the engine.
	Client bd.Client
	// NewEngine builds the engine once the session is open; the default runs a
	// refresh.Engine on Client.
	NewEngine func(bd.Session) Engine
	Tunables  *refresh.Tunables
	Actor     string

	Settings   config.Resolved
	Appearance appearance.Appearance
	Getenv     func(string) string
	Store      Persister
	// History is the persisted bar history; nil keeps it for the session.
	History History
	// Journal stores the events-journal opt-in answer per workspace; nil
	// keeps it for the session.
	Journal JournalStore
	// CopyKey copies a memory key to the clipboard; nil leaves the hint.
	CopyKey func(key string) tea.Cmd
	// Warnings and the appearance notice are shown once in the notice row.
	Warnings []string
	NoMouse  bool
	// Views fills the view slots 1-6; empty slots show the placeholder or the
	// "not available yet" hint.
	Views map[int]View

	// Startup facts for the failure screens.
	Dir          string
	BdPath       string
	BeadsDir     string
	BdashVersion string
	BdashViaBrew bool

	Now func() time.Time
	// RecheckMin and RecheckMax bound the back-off of the startup recheck.
	RecheckMin time.Duration
	RecheckMax time.Duration
}

func (o *Options) fill() {
	if o.Keys == nil {
		o.Keys = keys.Default()
	}
	if o.Getenv == nil {
		o.Getenv = func(string) string { return "" }
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.RecheckMin <= 0 {
		o.RecheckMin = 2 * time.Second
	}
	if o.RecheckMax <= 0 {
		o.RecheckMax = 30 * time.Second
	}
	if o.NewEngine == nil {
		client, tunables, actor := o.Client, o.Tunables, o.Actor
		o.NewEngine = func(s bd.Session) Engine {
			return refresh.New(refresh.Options{Client: client, Session: s, Tunables: tunables, Actor: actor})
		}
	}
}
