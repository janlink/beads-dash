package bd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// synthListJSON builds an envelope like bd list --all --limit 0 for n issues
// in epics of ten children with a blocking edge every fifth issue.
func synthListJSON(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"schema_version":1,"data":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		id := fmt.Sprintf("t-%05d", i)
		fmt.Fprintf(&b, `{"id":%q,"title":"Synthetic issue number %d with a realistic length title","description":"Some description text that is moderately long, for issue %d. It has enough words to matter for decoding.","status":"open","priority":%d,"issue_type":"task","assignee":"alice","owner":"tester@example.com","created_at":"2026-09-29T07:41:18Z","created_by":"janlink","updated_at":"2026-09-29T08:41:18Z","labels":["area:core","ui"],"comment_count":0,"dependency_count":1,"dependent_count":0`,
			id, i, i, i%5)
		var deps []string
		if i%10 != 0 {
			parent := fmt.Sprintf("t-%05d", i-i%10)
			fmt.Fprintf(&b, `,"parent":%q`, parent)
			deps = append(deps, fmt.Sprintf(`{"issue_id":%q,"depends_on_id":%q,"type":"parent-child","created_at":"2026-09-29T09:41:18Z","created_by":"janlink","metadata":"{}"}`, id, parent))
		}
		if i%5 == 4 {
			deps = append(deps, fmt.Sprintf(`{"issue_id":%q,"depends_on_id":"t-%05d","type":"blocks","created_at":"2026-09-29T09:41:18Z","created_by":"janlink","metadata":"{}"}`, id, i-1))
		}
		if len(deps) > 0 {
			b.WriteString(`,"dependencies":[` + strings.Join(deps, ",") + `]`)
		}
		b.WriteByte('}')
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

func synthReadyJSON(n int) []byte {
	var ready, blocked []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t-%05d", i)
		if i%5 == 4 {
			blocked = append(blocked, fmt.Sprintf(`{"id":%q,"title":"x","status":"open","priority":2,"blocked_by":[{"id":"t-%05d","title":"y","status":"open","priority":2}],"blocked_by_count":1}`, id, i-1))
		} else {
			ready = append(ready, fmt.Sprintf(`{"id":%q,"title":"x","status":"open","priority":2,"reason":"no blockers","dependency_count":0,"dependent_count":0}`, id))
		}
	}
	return []byte(`{"schema_version":1,"data":{"ready":[` + strings.Join(ready, ",") + `],"blocked":[` + strings.Join(blocked, ",") + `],"summary":{"total_ready":0,"total_blocked":0,"cycle_count":0}}}`)
}

func synthClient(n int) *ExecClient {
	return NewExec(ExecOptions{Runner: replay{
		"list":  {Stdout: synthListJSON(n)},
		"ready": {Stdout: synthReadyJSON(n)},
	}})
}

func BenchmarkFetchSnapshot5k(b *testing.B) {
	c := synthClient(5000)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FetchSnapshot(ctx, c, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFetchSnapshot20k(b *testing.B) {
	c := synthClient(20000)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FetchSnapshot(ctx, c, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// snapshotBudget5k is the most decode + derive at 5k issues may take, as the
// fastest of perfRuns runs. It sits well above the reference cost, so only a
// real regression such as a quadratic loop trips it.
const snapshotBudget5k = 250 * time.Millisecond

const perfRuns = 5

// fastestSnapshot is the shortest of runs snapshot builds over c.
func fastestSnapshot(t *testing.T, c Client, runs int) time.Duration {
	t.Helper()
	ctx := context.Background()
	best := time.Duration(1<<63 - 1)
	for range runs {
		start := time.Now()
		if _, err := FetchSnapshot(ctx, c, nil); err != nil {
			t.Fatal(err)
		}
		best = min(best, time.Since(start))
	}
	return best
}

// TestSnapshotBuildScalesLinearly holds at any machine load: it compares two
// sizes instead of measuring against a wall-clock budget.
func TestSnapshotBuildScalesLinearly(t *testing.T) {
	if raceEnabled || testing.Short() {
		t.Skip("timing ratios need an uninstrumented run")
	}
	small := fastestSnapshot(t, synthClient(5000), perfRuns)
	large := fastestSnapshot(t, synthClient(20000), perfRuns)
	ratio := float64(large) / float64(small)
	t.Logf("5k: %v, 20k: %v, ratio %.1f", small, large, ratio)
	if ratio >= 6 {
		t.Errorf("20k issues cost %.1fx the time of 5k, want under 6x for 4x the data", ratio)
	}
}

func TestSnapshotBuildWithinBudget5k(t *testing.T) {
	if os.Getenv("BDASH_PERF") != "1" || raceEnabled || testing.Short() {
		t.Skip("set BDASH_PERF=1 on an unloaded machine to check the wall-clock budget")
	}
	c := synthClient(5000)
	snap, err := FetchSnapshot(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Len() != 5000 || len(snap.ReadyIDs()) != 4000 || len(snap.BlockedIDs()) != 1000 {
		t.Fatalf("snapshot = %d issues, %d ready, %d blocked", snap.Len(), len(snap.ReadyIDs()), len(snap.BlockedIDs()))
	}
	per := fastestSnapshot(t, c, perfRuns)
	t.Logf("decode + derive at 5k issues: %v per snapshot", per)
	if per > snapshotBudget5k {
		t.Errorf("decode + derive at 5k issues took %v, budget %v", per, snapshotBudget5k)
	}
	if !json.Valid(synthListJSON(10)) {
		t.Fatal("synthetic list is not valid JSON")
	}
}
