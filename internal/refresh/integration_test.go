package refresh

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testbd"
)

const journaled = "journaled"

func TestMain(m *testing.M) {
	code := testbd.Run(m, testbd.Options{Recipes: map[string]testbd.Recipe{journaled: journalRecipe}})
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

func journalRecipe(e testbd.Env) error {
	if e.Version != "1.2.2" {
		if _, err := e.Bd("config", "set", "events-journal", "true"); err != nil {
			return err
		}
	}
	for _, t := range []string{"Alpha", "Beta"} {
		if _, err := e.Bd("create", "--silent", "--title="+t, "--type=task"); err != nil {
			return err
		}
	}
	return nil
}

// visibleWithin bounds the wait for a change made by another process; a
// change should show within about 2 s, the guard only keeps a broken run from
// hanging.
const visibleWithin = 20 * time.Second

// seenBound is the latency assertion, checked only with BDASH_PERF=1 in a
// serial run: the gate or debounce adds up to 2 s after the writing bd
// process has returned. Otherwise only the hang guard applies.
// BDASH_TIMEOUT_SCALE stretches it.
func seenBound() (time.Duration, bool) {
	if os.Getenv("BDASH_PERF") != "1" {
		return 0, false
	}
	scale := 1.0
	if v, err := strconv.ParseFloat(os.Getenv(config.EnvTimeoutScale), 64); err == nil && v > 0 {
		scale = v
	}
	return time.Duration(float64(3*time.Second) * scale), true
}

func TestIntegrationExternalUpdateIsSeenWithinSeconds(t *testing.T) {
	for _, v := range testbd.Versions {
		t.Run("bd-"+v, func(t *testing.T) {
			w := testbd.NewNamed(t, v, journaled)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := bd.NewExec(bd.ExecOptions{Bin: w.Bin, Dir: w.Dir})
			sess, err := bd.OpenSession(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			wantEvents := v != "1.2.2"
			if sess.EventsJournal != wantEvents {
				t.Fatalf("journal = %v", sess.EventsJournal)
			}

			eng := New(Options{Client: c, Session: sess})
			ups := eng.Updates()
			eng.Start(ctx)
			defer eng.Stop()

			first := waitFor(t, ups, func(u Update) bool { return u.Snapshot != nil })
			if first.Status.Mode == Events != wantEvents {
				t.Fatalf("mode = %v", first.Status.Mode)
			}
			id := ""
			for _, i := range first.Snapshot.IDs() {
				if is, _ := first.Snapshot.Issue(i); is.Title == "Alpha" {
					id = i
				}
			}
			if id == "" {
				t.Fatal("Alpha missing")
			}

			if _, err := w.Bd("--actor", "alice", "update", id, "--status", "in_progress", "--assignee", "alice"); err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			u := waitFor(t, ups, func(u Update) bool {
				for _, e := range u.Events {
					if e.IssueID == id && !e.Prefill {
						return true
					}
				}
				return false
			})
			elapsed := time.Since(started)
			t.Logf("bd %s: change visible after %v", v, elapsed.Round(10*time.Millisecond))
			if bound, on := seenBound(); on && elapsed > bound {
				t.Errorf("change took %v to show up, bound %v", elapsed, bound)
			}
			var ev model.Event
			for _, e := range u.Events {
				if e.IssueID == id && !e.Prefill {
					ev = e
				}
			}
			if ev.Kind != model.KindClaimed {
				t.Errorf("event = %+v", ev)
			}
			if wantEvents && ev.Actor != "alice" {
				t.Errorf("events mode must name the actor: %+v", ev)
			}
			if !wantEvents && ev.Actor != "" {
				t.Errorf("polling mode has no actor for a claim: %+v", ev)
			}
			if got := u.Highlights; len(got) != 1 || got[0] != id {
				t.Errorf("highlights = %v", got)
			}
			is, _ := u.Snapshot.Issue(id)
			if is.Assignee != "alice" || is.Status != "in_progress" {
				t.Errorf("snapshot not refreshed: %+v", is)
			}
			if wantEvents && !eng.Status().Following {
				t.Error("follower not running")
			}
			if !strings.Contains(sess.Version.Raw, v) {
				t.Errorf("version = %q", sess.Version.Raw)
			}
		})
	}
}

func waitFor(t *testing.T, ups <-chan Update, ok func(Update) bool) Update {
	t.Helper()
	guard := time.NewTimer(visibleWithin)
	defer guard.Stop()
	for {
		select {
		case u := <-ups:
			if ok(u) {
				return u
			}
		case <-guard.C:
			t.Fatalf("no matching update within %v", visibleWithin)
		}
	}
}
