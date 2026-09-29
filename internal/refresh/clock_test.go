package refresh

import (
	"testing"
	"time"
)

func recv(t *testing.T, tm Timer) (time.Time, bool) {
	t.Helper()
	select {
	case v := <-tm.C():
		return v, true
	default:
		return time.Time{}, false
	}
}

func TestFakeClockFiresTimersInOrderAtTheirDeadline(t *testing.T) {
	c := NewFakeClock(t0)
	a, b := c.NewTimer(0), c.NewTimer(0)
	a.Reset(5 * time.Second)
	b.Reset(2 * time.Second)
	c.Advance(time.Second)
	if _, ok := recv(t, b); ok {
		t.Fatal("fired early")
	}
	c.Advance(10 * time.Second)
	if v, ok := recv(t, b); !ok || !v.Equal(t0.Add(2*time.Second)) {
		t.Errorf("b fired at %v, %v", v, ok)
	}
	if v, ok := recv(t, a); !ok || !v.Equal(t0.Add(5*time.Second)) {
		t.Errorf("a fired at %v, %v", v, ok)
	}
	if !c.Now().Equal(t0.Add(11 * time.Second)) {
		t.Errorf("now = %v", c.Now())
	}
}

func TestFakeTimerResetAndStopLeaveNoStaleTick(t *testing.T) {
	c := NewFakeClock(t0)
	tm := c.NewTimer(0)
	tm.Reset(time.Second)
	c.Advance(time.Second)
	tm.Reset(time.Minute)
	if _, ok := recv(t, tm); ok {
		t.Error("stale tick after Reset")
	}
	tm.Stop()
	c.Advance(time.Hour)
	if _, ok := recv(t, tm); ok {
		t.Error("stopped timer fired")
	}
	tm.Reset(0)
	if _, ok := recv(t, tm); !ok {
		t.Error("Reset(0) must fire at once")
	}
}

func TestRealClockTimer(t *testing.T) {
	var c Clock = RealClock{}
	if c.Now().IsZero() {
		t.Fatal("zero now")
	}
	tm := c.NewTimer(time.Hour)
	if _, ok := recv(t, tm); ok {
		t.Error("new timer must start stopped")
	}
	tm.Reset(0)
	<-tm.C()
	tm.Reset(time.Hour)
	tm.Stop()
	if _, ok := recv(t, tm); ok {
		t.Error("stopped timer fired")
	}
}
