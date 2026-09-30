package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/model"
)

// Format is one of the three output formats.
type Format int

const (
	Markdown Format = iota
	JSON
	Text
)

// Formats lists the formats in the order the dialog cycles them.
var Formats = []Format{Markdown, JSON, Text}

func (f Format) String() string {
	switch f {
	case JSON:
		return "JSON"
	case Text:
		return "Text"
	case Markdown:
	}
	return "Markdown"
}

// Ext is the file extension without the dot.
func (f Format) Ext() string {
	switch f {
	case JSON:
		return "json"
	case Text:
		return "txt"
	case Markdown:
	}
	return "md"
}

// ParseFormat reads the names the command bar takes.
func ParseFormat(s string) (Format, bool) {
	switch strings.ToLower(s) {
	case "md", "markdown":
		return Markdown, true
	case "json":
		return JSON, true
	case "text", "txt":
		return Text, true
	}
	return Markdown, false
}

// Set names which issues an export holds.
type Set int

const (
	Current Set = iota
	Marked
	Scope
)

func (s Set) String() string {
	switch s {
	case Marked:
		return "Marked"
	case Scope:
		return "Scope"
	case Current:
	}
	return "Current"
}

// Input is everything a render needs. IDs are exported in the order given;
// IDs the snapshot does not hold are skipped.
type Input struct {
	Snap     *model.Snapshot
	Statuses model.Statuses
	IDs      []string
	Set      Set
	// Comments are attached to the issues that have some when WithComments is
	// set, oldest first.
	WithComments bool
	Comments     map[string][]bd.Comment
	// Prefix is the workspace's issue prefix, Query the scope query of a
	// Scope export.
	Prefix string
	Query  string
	Now    time.Time
	// Loc is the zone times are shown in; nil means UTC.
	Loc     *time.Location
	Version string
}

func (in Input) loc() *time.Location {
	if in.Loc == nil {
		return time.UTC
	}
	return in.Loc
}

// Render builds the export. Every format is UTF-8 with LF line endings and
// one trailing newline.
func Render(f Format, in Input) ([]byte, error) {
	ids := make([]string, 0, len(in.IDs))
	for _, id := range in.IDs {
		if _, ok := in.Snap.Issue(id); ok {
			ids = append(ids, id)
		}
	}
	in.IDs = ids
	switch f {
	case JSON:
		return renderJSON(in)
	case Text:
		return []byte(renderDoc(in, false)), nil
	case Markdown:
	}
	return []byte(renderDoc(in, true)), nil
}

// Name is the default file name: <id>.<ext> for one issue, else
// <prefix>-<marked|scope>-<YYYYMMDD-HHMM>.<ext>.
func Name(f Format, set Set, ids []string, prefix string, now time.Time) string {
	if len(ids) == 1 {
		return ids[0] + "." + f.Ext()
	}
	kind := strings.ToLower(set.String())
	if prefix == "" {
		prefix = "bdash"
	}
	return fmt.Sprintf("%s-%s-%s.%s", prefix, kind, now.Format("20060102-1504"), f.Ext())
}

// Size formats a byte count the way the dialog shows it.
func Size(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return unit(float64(n)/1024, "KB")
	}
	return unit(float64(n)/(1024*1024), "MB")
}

func unit(v float64, name string) string {
	if v >= 10 {
		return fmt.Sprintf("%.0f %s", v, name)
	}
	if s := fmt.Sprintf("%.1f", v); !strings.HasSuffix(s, ".0") {
		return s + " " + name
	}
	return fmt.Sprintf("%.0f %s", v, name)
}

// OverCap says why the clipboard cannot take n bytes, empty when it can.
func OverCap(n int) string {
	if n <= clipboard.MaxBytes {
		return ""
	}
	return fmt.Sprintf("%s, over the %d KB clipboard limit, save to file", Size(n), clipboard.MaxBytes/1024)
}
