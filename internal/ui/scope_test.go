package ui

import (
	"slices"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func snapshotWith(t testing.TB, edit func(is *model.Issue) bool) *model.Snapshot {
	t.Helper()
	snap := snapOf(t)
	var issues []model.Issue
	for _, id := range snap.IDs() {
		is, _ := snap.Issue(id)
		c := *is
		if edit(&c) {
			issues = append(issues, c)
		}
	}
	return model.NewSnapshot(issues, model.Readiness{}, uitest.T0)
}

func TestDeletedCurrentSelectsItsNeighbourInTheTree(t *testing.T) {
	a := treeApp(t, 100, 30)
	vis := slices.Clone(a.view().Visible(a.env()))
	gone := vis[4]
	a.sess.SetCurrent(gone)
	send(a, updateMsg{refresh.Update{
		Snapshot: snapshotWith(t, func(is *model.Issue) bool { return is.ID != gone }),
		Changed:  true, Status: liveStatus(),
	}})
	want := []string{vis[5], vis[3]}
	if !slices.Contains(want, a.sess.Current()) {
		t.Errorf("current %q, want a neighbour of %s: %v", a.sess.Current(), gone, want)
	}
}

func TestRefreshClosingCurrentRemembersAndReturns(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-7mt")
	closedNow := func(is *model.Issue) bool {
		if is.ID == "ws-7mt" {
			is.Status = "closed"
		}
		return true
	}
	send(a, updateMsg{refresh.Update{Snapshot: snapshotWith(t, closedNow), Changed: true, Status: liveStatus()}})
	if cur := a.sess.Current(); cur == "ws-7mt" || !a.view().Has(a.env(), cur) {
		t.Fatalf("current %q must leave the hidden closed issue", cur)
	}
	if a.away.orig != "ws-7mt" {
		t.Fatalf("away %+v does not remember the closed issue", a.away)
	}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Changed: true, Status: liveStatus()}})
	if a.sess.Current() != "ws-7mt" {
		t.Errorf("current %q, want ws-7mt back once it is shown again", a.sess.Current())
	}
}
