package model

import (
	"runtime"
	"sort"
	"sync"
	"time"
)

// Snapshot is the whole workspace as bdash last read it. It is immutable
// after [NewSnapshot] and safe to share between goroutines; the slices its
// methods return must not be modified.
type Snapshot struct {
	fetchedAt   time.Time
	issues      map[string]*Issue
	ids         []string
	children    map[string][]string
	dependents  map[string][]Edge
	ready       map[string]struct{}
	readyIDs    []string
	blocked     map[string][]string
	blockedIDs  []string
	fingerprint string
}

// NewSnapshot derives children, dependents and the fingerprint from bd's
// issues and readiness verdict. IDs in the verdict that are missing from
// issues are dropped; a duplicate issue ID keeps the last occurrence. The
// snapshot takes ownership of issues. Input order is irrelevant: every
// derived order is sorted by ID.
func NewSnapshot(issues []Issue, r Readiness, fetchedAt time.Time) *Snapshot {
	s := &Snapshot{
		fetchedAt: fetchedAt,
		issues:    make(map[string]*Issue, len(issues)),
		ready:     make(map[string]struct{}, len(r.Ready)),
		blocked:   make(map[string][]string, len(r.Blocked)),
	}
	for i := range issues {
		s.issues[issues[i].ID] = &issues[i]
	}
	s.ids = make([]string, 0, len(s.issues))
	for id := range s.issues {
		s.ids = append(s.ids, id)
	}
	sort.Strings(s.ids)

	s.deriveEdges()

	for _, id := range r.Ready {
		if _, ok := s.issues[id]; !ok {
			continue
		}
		if _, dup := s.ready[id]; !dup {
			s.ready[id] = struct{}{}
			s.readyIDs = append(s.readyIDs, id)
		}
	}
	sort.Strings(s.readyIDs)
	for id, by := range r.Blocked {
		if _, ok := s.issues[id]; !ok {
			continue
		}
		s.blocked[id] = sortedCopy(by)
		s.blockedIDs = append(s.blockedIDs, id)
	}
	sort.Strings(s.blockedIDs)

	s.fingerprint = s.computeFingerprint()
	return s
}

func (s *Snapshot) deriveEdges() {
	s.children = map[string][]string{}
	s.dependents = map[string][]Edge{}
	seenChild := map[[2]string]struct{}{}
	addChild := func(parent, child string) {
		if _, ok := s.issues[parent]; !ok || parent == child {
			return
		}
		if _, ok := s.issues[child]; !ok {
			return
		}
		k := [2]string{parent, child}
		if _, dup := seenChild[k]; dup {
			return
		}
		seenChild[k] = struct{}{}
		s.children[parent] = append(s.children[parent], child)
	}
	for _, id := range s.ids {
		is := s.issues[id]
		if is.Parent != "" {
			addChild(is.Parent, id)
		}
		for _, e := range is.Dependencies {
			if e.From == "" {
				e.From = id
			}
			if e.Type == EdgeParentChild {
				addChild(e.To, e.From)
				continue
			}
			if _, ok := s.issues[e.To]; ok {
				s.dependents[e.To] = append(s.dependents[e.To], e)
			}
		}
	}
	for _, c := range s.children {
		sort.Strings(c)
	}
	for _, d := range s.dependents {
		sort.Slice(d, func(i, j int) bool {
			if d[i].From != d[j].From {
				return d[i].From < d[j].From
			}
			return d[i].Type < d[j].Type
		})
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// FetchedAt is when the snapshot's bd calls finished.
func (s *Snapshot) FetchedAt() time.Time { return s.fetchedAt }

// Fingerprint identifies the snapshot's content, including fields bdash does
// not decode: equal for two snapshots of the same workspace state whatever
// order bd returned rows or keys in. Edge timestamps and derived counts do
// not count, since bd computes them per call.
func (s *Snapshot) Fingerprint() string { return s.fingerprint }

// Len is the number of issues.
func (s *Snapshot) Len() int { return len(s.ids) }

// IDs lists every issue ID sorted.
func (s *Snapshot) IDs() []string { return s.ids }

// Issue looks an issue up by ID. The result is read-only.
func (s *Snapshot) Issue(id string) (*Issue, bool) {
	is, ok := s.issues[id]
	return is, ok
}

// Children lists the direct children of id sorted by ID, from the parent
// field and parent-child edges alike.
func (s *Snapshot) Children(id string) []string { return s.children[id] }

// Dependents lists the non-parent edges that point at id, sorted by
// dependent ID. Edge.From is the dependent.
func (s *Snapshot) Dependents(id string) []Edge { return s.dependents[id] }

// IsContainer reports whether the issue has children.
func (s *Snapshot) IsContainer(id string) bool { return len(s.children[id]) > 0 }

// IsReady reports whether bd listed the issue as ready.
func (s *Snapshot) IsReady(id string) bool {
	_, ok := s.ready[id]
	return ok
}

// ReadyIDs lists bd's ready issues sorted by ID.
func (s *Snapshot) ReadyIDs() []string { return s.readyIDs }

// IsBlocked reports whether bd listed the issue as blocked by a dependency.
func (s *Snapshot) IsBlocked(id string) bool {
	_, ok := s.blocked[id]
	return ok
}

// BlockedBy lists the IDs bd names as blocking id, sorted.
func (s *Snapshot) BlockedBy(id string) []string { return s.blocked[id] }

// BlockedIDs lists bd's blocked issues sorted by ID.
func (s *Snapshot) BlockedIDs() []string { return s.blockedIDs }

// Present returns the presentation status of an issue under the given
// status table. It reports Other for an unknown ID.
func (s *Snapshot) Present(id string, st Statuses) Presentation {
	is, ok := s.issues[id]
	if !ok {
		return Presentation{Status: Other}
	}
	return Present(is.Status, s.IsBlocked(id), st)
}

// Count is closed out of total.
type Count struct {
	Closed int
	Total  int
}

// Progress is how far a container has come: closed direct children out of
// all direct children, and the same over all descendants for the bar.
type Progress struct {
	Direct      Count
	Descendants Count
}

// Progress computes progress for an issue with children; ok is false when it
// has none. Closed means the status category is done, so Frozen and Other
// children count as not closed.
func (s *Snapshot) Progress(id string, st Statuses) (p Progress, ok bool) {
	kids := s.children[id]
	if len(kids) == 0 {
		return Progress{}, false
	}
	closed := func(c string) bool {
		return st.Category(s.issues[c].Status) == CategoryDone
	}
	for _, c := range kids {
		p.Direct.Total++
		if closed(c) {
			p.Direct.Closed++
		}
	}
	seen := map[string]struct{}{id: {}}
	stack := append([]string(nil), kids...)
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		p.Descendants.Total++
		if closed(c) {
			p.Descendants.Closed++
		}
		stack = append(stack, s.children[c]...)
	}
	return p, true
}

func (s *Snapshot) issueDigest(is *Issue) string {
	var h fingerprintHasher
	h.init()
	h.strs(is.ID, is.Title, is.Description, is.Design, is.AcceptanceCriteria, is.Notes,
		is.Status, is.IssueType, is.Assignee, is.Owner, is.CreatedBy, is.Parent,
		is.CloseReason, is.ExternalRef)
	h.ints(int64(is.Priority), int64(is.EstimatedMinutes), int64(is.CommentCount))
	for _, t := range []time.Time{is.CreatedAt, is.UpdatedAt, is.StartedAt, is.ClosedAt, is.DueAt} {
		h.ints(t.UnixNano())
	}
	h.list(sortedCopy(is.Labels))
	edges := make([]string, len(is.Dependencies))
	for i, e := range is.Dependencies {
		edges[i] = e.From + "\x1f" + e.To + "\x1f" + e.Type
	}
	sort.Strings(edges)
	h.list(edges)
	h.str(canonicalRaw(is.Raw))
	return h.sum()
}

// computeFingerprint hashes each issue independently, spread over the CPUs
// because canonicalising bd's JSON dominates the cost at thousands of issues,
// then combines the digests in ID order.
func (s *Snapshot) computeFingerprint() string {
	digests := make([]string, len(s.ids))
	workers := min(runtime.GOMAXPROCS(0), len(s.ids)/64+1)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := w; i < len(s.ids); i += workers {
				digests[i] = s.issueDigest(s.issues[s.ids[i]])
			}
		}()
	}
	wg.Wait()

	var h fingerprintHasher
	h.init()
	h.list(digests)
	h.str("ready")
	h.list(s.readyIDs)
	h.str("blocked")
	for _, id := range s.blockedIDs {
		h.str(id)
		h.list(s.blocked[id])
	}
	return h.sum()
}
