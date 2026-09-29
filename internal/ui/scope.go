package ui

import (
	"slices"
	"strings"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/state"
)

// away remembers the current issue a narrowing scope pushed out of the view:
// orig is where the viewer was, landed is where the shell moved the current
// issue. It applies only while the current issue is still landed.
type away struct{ orig, landed string }

func (a *App) setScope(s model.Scope) {
	a.scope = s
	a.sess.ScopeActive = s.Active()
	if a.rend != nil {
		a.rend.SetMatch(s.MatchTerms())
	}
}

// clearedScope is the scope without search or filter tokens, status tokens
// included; only the show_closed default survives.
func (a *App) clearedScope() model.Scope { return model.ParseScope("", a.scope.ShowClosed()) }

// applyScope makes s the scope and keeps the current issue in the view: it
// moves to the nearest issue still shown, and returns to the issue it left
// when the scope widens again.
func (a *App) applyScope(s model.Scope) {
	if s.Key() == a.scope.Key() {
		return
	}
	old := slices.Clone(a.view().Visible(a.env()))
	a.setScope(s)
	a.reconcile(old)
}

func (a *App) reconcile(old []string) {
	if a.snap == nil {
		return
	}
	env := a.env()
	v := a.view()
	cur := a.sess.Current()
	if a.away.orig != "" {
		switch {
		case cur != a.away.landed:
			a.away = away{}
		case v.Has(env, a.away.orig):
			a.sess.SetCurrent(a.away.orig)
			a.away = away{}
			return
		}
	}
	if cur == "" || v.Has(env, cur) {
		return
	}
	next := state.NearestSurvivor(old, cur, func(id string) bool { return v.Has(env, id) })
	if next == "" {
		if vis := v.Visible(env); len(vis) > 0 {
			next = vis[0]
		}
	}
	if next == "" || next == cur {
		return
	}
	if _, ok := a.snap.Issue(cur); !ok {
		a.sess.SetCurrent(next)
		return
	}
	orig := cur
	if a.away.orig != "" {
		orig = a.away.orig
	}
	a.away = away{orig, next}
	a.sess.SetCurrent(next)
}

// resolveID finds the issue a typed ID names: exactly, or without the
// workspace prefix.
func (a *App) resolveID(id string) (string, bool) {
	if a.snap == nil || id == "" {
		return "", false
	}
	if _, ok := a.snap.Issue(id); ok {
		return id, true
	}
	if p := a.bds.Workspace.Prefix; p != "" {
		if _, ok := a.snap.Issue(p + "-" + id); ok {
			return p + "-" + id, true
		}
	}
	return "", false
}

// jumpTo makes id the current issue and pushes where the viewer was on the
// back stack. An issue the scope hides clears the scope; one the view cannot
// show at all opens in the first view that can.
func (a *App) jumpTo(id string) bool {
	if a.snap == nil {
		return false
	}
	if _, ok := a.snap.Issue(id); !ok {
		return false
	}
	if !a.visible(id) && a.scope.Active() {
		a.applyScope(a.clearedScope())
	}
	if a.visible(id) {
		a.sess.Jump(id)
		return true
	}
	for n, v := range a.views {
		if v != nil && v.Has(a.env(), id) {
			a.show(n+1, id)
			return true
		}
	}
	a.sess.Jump(id)
	return true
}

// idsWith lists up to limit issue IDs that start with prefix, ignoring case.
func (a *App) idsWith(prefix string, limit int) []string {
	if a.snap == nil {
		return nil
	}
	low := strings.ToLower(prefix)
	var out []string
	for _, id := range a.snap.IDs() {
		if strings.HasPrefix(strings.ToLower(id), low) {
			out = append(out, id)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}
