package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func TestKindsAreThirteenUniqueNames(t *testing.T) {
	names := model.KindNames()
	if len(names) != 13 {
		t.Fatalf("got %d kinds, want 13", len(names))
	}
	seen, labels := map[string]bool{}, map[string]bool{}
	for _, k := range model.Kinds() {
		if seen[string(k)] {
			t.Errorf("duplicate kind %q", k)
		}
		seen[string(k)] = true
		if labels[k.Label()] || k.Label() == "" {
			t.Errorf("kind %q has an empty or duplicate label %q", k, k.Label())
		}
		labels[k.Label()] = true
		if got, ok := model.ParseKind(string(k)); !ok || got != k {
			t.Errorf("ParseKind(%q) = %q, %v", k, got, ok)
		}
	}
	if _, ok := model.ParseKind("nope"); ok {
		t.Error("ParseKind accepted an unknown name")
	}
	if got := model.Kind("odd").Label(); got != "odd" {
		t.Errorf("unknown kind label = %q", got)
	}
}

func TestKindSet(t *testing.T) {
	s := model.KindSetOf("closed", "ready", "bogus")
	if !s.Has(model.KindClosed) || !s.Has(model.KindBecameReady) || s.Has(model.KindCreated) || len(s) != 2 {
		t.Errorf("set = %v", s)
	}
}

func ev(k model.Kind, id string, at time.Time) model.Event {
	return model.Event{Kind: k, IssueID: id, Time: at}
}

func TestRingKeepsNewestFiveHundred(t *testing.T) {
	var r model.Ring
	for i := 0; i < model.RingCapacity+20; i++ {
		r.Add(ev(model.KindEdited, "x", t0.Add(time.Duration(i)*time.Second)))
	}
	if r.Len() != model.RingCapacity {
		t.Fatalf("len = %d", r.Len())
	}
	got := r.Newest()
	if !got[0].Time.Equal(t0.Add(time.Duration(model.RingCapacity+19)*time.Second)) ||
		!got[len(got)-1].Time.Equal(t0.Add(20*time.Second)) {
		t.Errorf("newest first %v ... %v", got[0].Time, got[len(got)-1].Time)
	}
}

func TestRingMergeOrdersByTimeAndTrimsOldest(t *testing.T) {
	var r model.Ring
	r.Add(ev(model.KindClosed, "live1", t0.Add(10*time.Second)), ev(model.KindClosed, "live2", t0.Add(20*time.Second)))
	r.Merge()
	r.Merge(ev(model.KindCreated, "old2", t0.Add(2*time.Second)), ev(model.KindCreated, "old1", t0.Add(1*time.Second)))
	var ids []string
	for _, e := range r.Newest() {
		ids = append(ids, e.IssueID)
	}
	if want := []string{"live2", "live1", "old2", "old1"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order = %v, want %v", ids, want)
	}

	var big model.Ring
	old := make([]model.Event, model.RingCapacity)
	for i := range old {
		old[i] = ev(model.KindCreated, "o", t0.Add(time.Duration(i)*time.Second))
	}
	big.Merge(old...)
	big.Add(ev(model.KindClosed, "new", t0.Add(time.Hour)))
	if big.Len() != model.RingCapacity || big.Newest()[0].IssueID != "new" {
		t.Errorf("len %d newest %v", big.Len(), big.Newest()[0])
	}
}
