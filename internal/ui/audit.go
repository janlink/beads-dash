package ui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/ui/detail"
)

var errNoEngine = errors.New("bd is not available")

// auditMsg carries a finished audit read to the panel.
type auditMsg struct{ res detail.FetchResult }

// auditJobs starts the bd reads the audit trail wants and cancels those the
// panel no longer wants.
func (a *App) auditJobs(in detail.Input) tea.Cmd {
	start, cancel := a.panel.PlanAudit(in)
	for _, f := range cancel {
		a.endFetch(f.Seq)
	}
	if len(start) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(start))
	for _, f := range start {
		if a.eng == nil {
			a.panel.ApplyAudit(detail.FetchResult{Fetch: f, Err: errNoEngine})
			continue
		}
		ctx, stop := context.WithCancel(context.Background())
		if a.fetching == nil {
			a.fetching = map[int]context.CancelFunc{}
		}
		a.fetching[f.Seq] = stop
		eng := a.eng
		cmds = append(cmds, func() tea.Msg { return auditMsg{fetchAudit(ctx, eng, f)} })
	}
	return tea.Batch(cmds...)
}

func (a *App) endFetch(seq int) {
	if stop := a.fetching[seq]; stop != nil {
		stop()
		delete(a.fetching, seq)
	}
}

// fetchAudit runs one read in the queue. When ctx ends first, Do returns
// before fn has finished, so the captured results are only read after a nil
// error.
func fetchAudit(ctx context.Context, eng Engine, f detail.Fetch) detail.FetchResult {
	var (
		comments []bd.Comment
		history  []bd.HistoryEntry
	)
	err := eng.Do(ctx, func(ctx context.Context, c bd.Client) error {
		var err error
		switch f.Kind {
		case detail.AuditComments:
			comments, err = c.Comments(ctx, f.ID)
		case detail.AuditHistory:
			history, err = c.History(ctx, f.ID)
		}
		return err
	})
	if err != nil {
		return detail.FetchResult{Fetch: f, Err: err}
	}
	return detail.FetchResult{Fetch: f, Comments: comments, History: history}
}
