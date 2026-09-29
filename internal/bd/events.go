package bd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
)

const codeJournalTruncated = "events_journal_truncated"

// EventsFollow implements [Client]: bd events tail --since N --follow. The
// stream lives until ctx ends or Close is called; there is no timeout.
// The runner must be a [Streamer], as [ExecRunner] is.
func (c *ExecClient) EventsFollow(ctx context.Context, since int64) (EventStream, error) {
	st, ok := c.runner.(Streamer)
	if !ok {
		return nil, ErrNotImplemented
	}
	if since < 0 {
		since = 0
	}
	const cmd = "events tail"
	ps, err := st.Stream(ctx, []string{"events", "tail", "--since", strconv.FormatInt(since, 10), "--follow", "--json"})
	if err != nil {
		return nil, c.runError(ctx, ctx, cmd, err)
	}
	return &eventStream{ps: ps, r: bufio.NewReaderSize(ps, 64<<10), since: since}, nil
}

type eventStream struct {
	ps    ProcessStream
	r     *bufio.Reader
	since int64
	// rest collects output that is not a record, such as a multi-line
	// pretty-printed error, for judging the exit.
	rest  bytes.Buffer
	final error
	once  sync.Once
}

// Next implements [EventStream].
func (s *eventStream) Next() (Event, error) {
	for {
		if s.final != nil {
			return Event{}, s.final
		}
		line, err := s.r.ReadBytes('\n')
		if body := bytes.TrimSpace(line); len(body) > 0 {
			ev, trunc, isRecord := s.parseLine(body)
			switch {
			case trunc != nil:
				s.final = trunc
				s.stop()
				return Event{}, trunc
			case isRecord:
				return ev, nil
			}
		}
		if err != nil {
			s.final = s.finish(err)
			return Event{}, s.final
		}
	}
}

func (s *eventStream) parseLine(body []byte) (ev Event, trunc *JournalTruncatedError, isRecord bool) {
	if body[0] == '{' {
		if e, ok := parseEventRecord(body); ok {
			return e, nil, true
		}
		if t, ok := parseTruncated(body, s.since); ok {
			return Event{}, t, false
		}
	}
	s.rest.Write(body)
	s.rest.WriteByte('\n')
	return Event{}, nil, false
}

// finish judges the end of the output. A read error other than EOF means
// the pipe broke or was closed.
func (s *eventStream) finish(readErr error) error {
	res, waitErr := s.ps.Wait()
	if t, ok := parseTruncated(s.rest.Bytes(), s.since); ok {
		return t
	}
	switch {
	case waitErr != nil:
		return waitErr
	case res.ExitCode != 0:
		e := &Error{Class: ClassTransient, Command: "events tail", ExitCode: res.ExitCode, Stderr: clip(res.Stderr)}
		if f, found := parseFailure(s.rest.Bytes(), res.Stderr); found {
			e.Code, e.Message = f.Code, f.Message
		}
		return e
	case !errors.Is(readErr, io.EOF):
		return readErr
	}
	return io.EOF
}

// Close implements [EventStream].
func (s *eventStream) Close() error {
	s.stop()
	return nil
}

func (s *eventStream) stop() { s.once.Do(func() { _ = s.ps.Close() }) }

type wireEvent struct {
	Seq     int64           `json:"seq"`
	Ts      lenientTime     `json:"ts"`
	Op      string          `json:"op"`
	IssueID string          `json:"issue_id"`
	Actor   string          `json:"actor"`
	Issue   json.RawMessage `json:"issue"`
}

// parseEventRecord decodes one journal line. A line without seq and op is
// not a record.
func parseEventRecord(line []byte) (Event, bool) {
	var w wireEvent
	if json.Unmarshal(line, &w) != nil || w.Seq <= 0 || w.Op == "" {
		return Event{}, false
	}
	ev := Event{
		Seq: w.Seq, Time: w.Ts.time(), Op: w.Op, IssueID: w.IssueID, Actor: w.Actor,
		Raw: json.RawMessage(bytes.Clone(line)),
	}
	if len(w.Issue) > 0 && !bytes.Equal(w.Issue, []byte("null")) {
		var wi wireIssue
		var flag struct {
			IsBlocked bool `json:"is_blocked"`
		}
		if json.Unmarshal(w.Issue, &wi) == nil {
			is := wi.issue(w.Issue)
			ev.Issue = &is
			if json.Unmarshal(w.Issue, &flag) == nil {
				ev.Blocked = flag.IsBlocked
			}
		}
	}
	return ev, true
}

// parseTruncated recognises the events_journal_truncated failure, flat or
// envelope-wrapped, on one line or pretty-printed.
func parseTruncated(b []byte, since int64) (*JournalTruncatedError, bool) {
	body := bytes.TrimSpace(b)
	if len(body) == 0 || body[0] != '{' {
		return nil, false
	}
	type fields struct {
		Code  string `json:"code"`
		Since *int64 `json:"since"`
		Floor int64  `json:"floor"`
		Head  int64  `json:"head"`
	}
	var top struct {
		fields
		Data *fields `json:"data"`
	}
	if json.Unmarshal(body, &top) != nil {
		return nil, false
	}
	f := top.fields
	if top.Data != nil && top.Data.Code != "" {
		f = *top.Data
	}
	if f.Code != codeJournalTruncated {
		return nil, false
	}
	t := &JournalTruncatedError{Since: since, Floor: f.Floor, Head: f.Head}
	if f.Since != nil {
		t.Since = *f.Since
	}
	return t, true
}
