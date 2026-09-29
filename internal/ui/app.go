// Package ui holds the Bubble Tea program: shell, views and dialogs.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var quitKey = key.NewBinding(key.WithKeys("q", "ctrl+c"))

// App is the root model.
type App struct {
	style lipgloss.Style
}

// New returns the root model.
func New() App {
	return App{style: lipgloss.NewStyle().Bold(true)}
}

// Init implements tea.Model.
func (App) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, quitKey) {
		return a, tea.Quit
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() tea.View {
	v := tea.NewView(a.style.Render("bdash") + "\npress q to quit\n")
	v.AltScreen = true
	return v
}
