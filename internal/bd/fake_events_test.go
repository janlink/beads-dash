package bd

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestFakeJournalStreamsFromSince(t *testing.T) {
	f := NewFake()
	f.AppendEvents(Event{Op: "create", IssueID: "a"}, Event{Op: "update", IssueID: "a"}, Event{Op: "close", IssueID: "a"})
	s, err := f.EventsFollow(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range []int64{2, 3} {
		ev, err := s.Next()
		if err != nil || ev.Seq != seq {
			t.Fatalf("Next = %+v, %v; want seq %d", ev, err, seq)
		}
	}
	got := make(chan Event, 1)
	go func() {
		ev, _ := s.Next()
		got <- ev
		_, _ = s.Next()
	}()
	f.Emit(Event{Op: "delete", IssueID: "a"})
	if ev := <-got; ev.Seq != 4 || ev.Op != "delete" {
		t.Errorf("Next = %+v", ev)
	}
	if got := f.FollowStarts(); !reflect.DeepEqual(got, []int64{1}) {
		t.Errorf("FollowStarts = %v", got)
	}
	if f.OpenFollowers() != 1 {
		t.Error("stream should be open")
	}
	_ = s.Close()
	if f.OpenFollowers() != 0 {
		t.Error("stream should be closed")
	}
	if _, err := s.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next after Close = %v", err)
	}
}

func TestFakeKillFollowersDeliversErrorAfterRecords(t *testing.T) {
	f := NewFake()
	s, _ := f.EventsFollow(context.Background(), 0)
	f.AppendEvents(Event{Op: "create", IssueID: "a"})
	boom := errors.New("boom")
	done := make(chan struct{})
	go func() {
		defer close(done)
		if ev, err := s.Next(); err != nil || ev.Seq != 1 {
			t.Errorf("Next = %+v, %v", ev, err)
		}
		if _, err := s.Next(); !errors.Is(err, boom) {
			t.Errorf("Next = %v, want boom", err)
		}
		_ = s.Close()
	}()
	f.KillFollowers(boom)
	<-done
}

func TestFakeTruncateFillsSince(t *testing.T) {
	f := NewFake()
	s, _ := f.EventsFollow(context.Background(), 4)
	done := make(chan error, 1)
	go func() {
		_, err := s.Next()
		_ = s.Close()
		done <- err
	}()
	f.Truncate(9, 30)
	var tr *JournalTruncatedError
	if err := <-done; !errors.As(err, &tr) || tr.Since != 4 || tr.Floor != 9 || tr.Head != 30 {
		t.Errorf("err = %v", err)
	}
}

func TestFakeStreamClosesWithContext(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := f.EventsFollow(ctx, 0); err != nil {
		t.Fatal(err)
	}
	cancel()
	f.DrainFollowers()
	if f.OpenFollowers() != 0 {
		t.Error("stream survived its context")
	}
}

func TestFakeHookRunsBeforeInjectedFailure(t *testing.T) {
	f := NewFake()
	var seen []string
	f.SetHook(func(m string) { seen = append(seen, m) })
	f.FailWith("List", errors.New("x"))
	_, _ = f.List(context.Background())
	_, _ = f.Ready(context.Background())
	if !reflect.DeepEqual(seen, []string{"List", "Ready"}) {
		t.Errorf("hook saw %v", seen)
	}
}
