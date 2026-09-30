package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
)

// wireComment is the shape bd show --include-comments gives a comment.
type wireComment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// renderJSON is always an array of bd's own list objects, kept as bd wrote
// them so fields bdash does not know survive. Comments join as the comments
// key.
func renderJSON(in Input) ([]byte, error) {
	var raw bytes.Buffer
	raw.WriteByte('[')
	for i, id := range in.IDs {
		is, _ := in.Snap.Issue(id)
		obj := bytes.TrimSpace(is.Raw)
		if len(obj) == 0 {
			return nil, fmt.Errorf("%s has no bd JSON to export", id)
		}
		if in.WithComments && len(in.Comments[id]) > 0 {
			var err error
			if obj, err = withComments(obj, in.Comments[id]); err != nil {
				return nil, err
			}
		}
		if i > 0 {
			raw.WriteByte(',')
		}
		raw.Write(obj)
	}
	raw.WriteByte(']')
	var out bytes.Buffer
	if err := json.Indent(&out, raw.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func withComments(obj []byte, list []bd.Comment) ([]byte, error) {
	sorted := slices.Clone(list)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	wire := make([]wireComment, len(sorted))
	for i, c := range sorted {
		wire[i] = wireComment{c.ID, c.IssueID, c.Author, c.Text, c.CreatedAt.UTC()}
	}
	enc, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	body := bytes.TrimSpace(obj[:len(obj)-1])
	sep := []byte(",")
	if bytes.HasSuffix(body, []byte("{")) {
		sep = nil
	}
	out := append([]byte(nil), body...)
	out = append(out, sep...)
	out = append(out, `"comments":`...)
	out = append(out, enc...)
	return append(out, '}'), nil
}
