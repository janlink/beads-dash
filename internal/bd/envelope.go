package bd

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// SchemaVersion is the only JSON schema_version bdash understands.
const SchemaVersion = 1

type envelope struct {
	SchemaVersion *int            `json:"schema_version"`
	Data          json.RawMessage `json:"data"`
}

// unwrap returns the payload of a --json answer. It reads the envelope
// {"schema_version":1,"data":...}, and tolerates bd's legacy output (bare
// arrays, objects with an injected schema_version). A schema_version other
// than 1 is a hard stop.
func unwrap(command string, stdout []byte) (json.RawMessage, error) {
	body := bytes.TrimSpace(stdout)
	if len(body) == 0 {
		return nil, &Error{Class: ClassTransient, Command: command, Message: "empty output"}
	}
	if body[0] == '[' {
		return body, nil
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &Error{Class: ClassTransient, Command: command, Message: "output is not JSON", Err: err}
	}
	if env.SchemaVersion != nil && *env.SchemaVersion != SchemaVersion {
		return nil, &Error{
			Class: ClassUnsupported, Command: command,
			Message: fmt.Sprintf("schema_version %d, bdash understands %d", *env.SchemaVersion, SchemaVersion),
		}
	}
	if len(env.Data) == 0 {
		return body, nil
	}
	return env.Data, nil
}

// failure is what bd's JSON error output says.
type failure struct {
	Code    string
	Message string
	Skew    bool
}

// parseFailure looks for a JSON error object in bd's output streams, in
// order. bd puts the object on stdout for where and show and on stderr for
// schema skew; both may be envelope-wrapped.
func parseFailure(streams ...[]byte) (f failure, found bool) {
	for _, s := range streams {
		body := bytes.TrimSpace(s)
		if len(body) == 0 || body[0] != '{' {
			continue
		}
		var top struct {
			Error      string          `json:"error"`
			Message    string          `json:"message"`
			SchemaSkew json.RawMessage `json:"schema_skew"`
			Data       *struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &top) != nil {
			continue
		}
		f = failure{Code: top.Error, Message: top.Message, Skew: len(top.SchemaSkew) > 0}
		if top.Data != nil && top.Data.Error != "" {
			f.Code, f.Message = top.Data.Error, top.Data.Message
		}
		if f.Code != "" || f.Skew {
			return f, true
		}
	}
	return failure{}, false
}
