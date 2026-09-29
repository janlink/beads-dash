package bd

import (
	"context"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func TestOpenSession(t *testing.T) {
	f := NewFake()
	f.SetStatuses(model.NewStatuses([]model.StatusInfo{{Name: "review", Category: model.CategoryWIP, Custom: true}}))
	f.SetConfig("events-journal", "true")
	s, err := OpenSession(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version.Raw != "1.3.0" || s.Workspace.Prefix != "f" || s.Statuses.Category("review") != model.CategoryWIP ||
		len(s.Types) == 0 || !s.EventsJournal {
		t.Errorf("session = %+v", s)
	}
}

func TestOpenSessionSkipsJournalProbeWithoutCapability(t *testing.T) {
	f := NewFake()
	f.SetVersion("1.2.2")
	f.SetConfig("events-journal", "true")
	s, err := OpenSession(context.Background(), f)
	if err != nil || s.EventsJournal {
		t.Errorf("session = %+v, %v", s, err)
	}
	for _, c := range f.Calls() {
		if c == "ConfigGet" {
			t.Error("bd 1.2.2 has no journal, ConfigGet must not be called")
		}
	}
}

func TestOpenSessionJournalProbeFailureIsNotFatal(t *testing.T) {
	f := NewFake()
	f.FailWith("ConfigGet", &Error{Class: ClassTransient})
	s, err := OpenSession(context.Background(), f)
	if err != nil || s.EventsJournal {
		t.Errorf("session = %+v, %v", s, err)
	}
}

func TestOpenSessionStopsAtFirstFailure(t *testing.T) {
	steps := []struct {
		method string
		class  Class
	}{
		{"Version", ClassBdMissing}, {"Where", ClassNotWorkspace}, {"Statuses", ClassTransient}, {"Types", ClassTimeout},
	}
	for _, st := range steps {
		f := NewFake()
		f.FailWith(st.method, &Error{Class: st.class})
		_, err := OpenSession(context.Background(), f)
		if !IsClass(err, st.class) {
			t.Errorf("%s failing: %v", st.method, err)
		}
	}
	f := NewFake()
	f.SetVersion("1.0.0")
	s, err := OpenSession(context.Background(), f)
	if !IsClass(err, ClassUnsupported) || s.Version.Raw != "1.0.0" {
		t.Errorf("unsupported version: %+v, %v", s.Version, err)
	}
	for _, c := range f.Calls() {
		if c == "Where" {
			t.Error("Where must not run after an unsupported version")
		}
	}
}

func TestSessionRefreshMeta(t *testing.T) {
	f := NewFake()
	s, _ := OpenSession(context.Background(), f)
	f.SetStatuses(model.NewStatuses([]model.StatusInfo{{Name: "later", Category: model.CategoryFrozen, Custom: true}}))
	if err := s.RefreshMeta(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if s.Statuses.Category("later") != model.CategoryFrozen {
		t.Error("statuses not reloaded")
	}
	f.FailWith("Types", &Error{Class: ClassTransient})
	f.SetStatuses(model.NewStatuses(nil))
	if err := s.RefreshMeta(context.Background(), f); err == nil {
		t.Fatal("expected failure")
	}
	if s.Statuses.Category("later") != model.CategoryFrozen {
		t.Error("a failed reload must keep the old tables")
	}
}

func TestJournalEnabledValues(t *testing.T) {
	f := NewFake()
	caps := capabilitiesFor(Version{1, 3, 0})
	for value, want := range map[string]bool{"true": true, "1": true, "false": false, "": false, "off": false} {
		f.SetConfig("events-journal", value)
		if got, err := journalEnabled(context.Background(), f, caps); err != nil || got != want {
			t.Errorf("value %q = %v, %v", value, got, err)
		}
	}
	f.FailWith("ConfigGet", &Error{Class: ClassTransient})
	if _, err := journalEnabled(context.Background(), f, caps); err == nil {
		t.Error("error must surface")
	}
}
