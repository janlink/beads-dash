package ui

import (
	"fmt"
	"testing"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/testgolden"
)

var formSizes = [][2]int{{60, 16}, {80, 24}}

func TestIssueFormGoldens(t *testing.T) {
	scenes := []struct {
		name string
		set  func(*writeRig)
	}{
		{"create", func(r *writeRig) {
			r.key("n")
			r.text("Retry the webhook")
		}},
		{"edit_with_changes", func(r *writeRig) {
			r.a.sess.SetCurrent("ws-9qe")
			r.key("e")
			r.text(" again")
			r.form().f.FocusKey(fPriority)
			r.form().f.Field(fPriority).Set("P1")
		}},
		{"advanced_open", func(r *writeRig) {
			r.a.sess.SetCurrent("ws-9qe")
			r.key("e")
			r.form().f.FocusKey("advanced")
			r.key("enter")
		}},
		{"conflict", func(r *writeRig) {
			r.a.sess.SetCurrent("ws-9qe")
			r.key("e")
			r.text("!")
			r.external("ws-9qe", bd.UpdateSpec{Title: ptr("Retries, reworded"), Priority: ptr(3)})
		}},
		{"error", func(r *writeRig) {
			r.a.sess.SetCurrent("ws-9qe")
			r.fake.FailWith("Update", &bd.Error{Message: "database is locked by another writer; try again in a moment"})
			r.key("e")
			r.text("!")
			r.key("ctrl+s")
		}},
	}
	for _, s := range scenes {
		for _, sz := range formSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", s.name, sz[0], sz[1]), func(t *testing.T) {
				r := newRig(t, sz[0], sz[1])
				s.set(r)
				testgolden.Equal(t, screen(r.a))
			})
		}
	}
}
