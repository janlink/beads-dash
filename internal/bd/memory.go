package bd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrMemoryGone is what Forget returns for a key bd does not hold: someone
// else already forgot it. It is not a failure of the write.
var ErrMemoryGone = errors.New("bd: memory already gone")

// CheckMemoryKey reports why key cannot name a memory: empty after trimming,
// or holding a newline or another control character. Spaces, dots, capitals
// and a leading dash are fine; bd stores them verbatim.
func CheckMemoryKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("key is empty")
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return fmt.Errorf("key holds a control character")
		}
	}
	return nil
}

func memoryRejected(cmd string, res Result, msg string) error {
	return &Error{Class: ClassRejected, Command: cmd, ExitCode: res.ExitCode, Message: msg, Stderr: clip(res.Stderr)}
}

// memoryAnswer is the data object of remember and forget. bd prints its
// booleans as strings.
type memoryAnswer struct {
	Action  string `json:"action"`
	Key     string `json:"key"`
	Deleted string `json:"deleted"`
	Found   string `json:"found"`
	Error   string `json:"error"`
}

func (c *ExecClient) memoryWrite(ctx context.Context, argv ...string) (Result, memoryAnswer, string, error) {
	res, r, cmd, err := c.write(ctx, argv...)
	if err != nil {
		return res, memoryAnswer{}, cmd, err
	}
	var a memoryAnswer
	if data, uerr := unwrap(cmd, res.Stdout); uerr == nil {
		_ = json.Unmarshal(data, &a)
	} else if IsClass(uerr, ClassUnsupported) {
		return res, a, cmd, uerr
	}
	if a.Error != "" && r.msg == "" {
		r.msg = a.Error
	}
	if a.Found != "false" && (r.exit != 0) {
		return res, a, cmd, c.judge(cmd, res, r, nil)
	}
	return res, a, cmd, nil
}

// Remember implements [Client]. It always passes --key and --, so an
// existing key is overwritten instead of recalled and content that starts
// with a dash is stored as written. The answer must say "remembered" (a new key)
// or "updated" (an existing one) for the requested key.
func (c *ExecClient) Remember(ctx context.Context, key, content string) error {
	if err := CheckMemoryKey(key); err != nil {
		return &Error{Class: ClassRejected, Command: "remember", Message: err.Error()}
	}
	if strings.TrimSpace(content) == "" {
		return &Error{Class: ClassRejected, Command: "remember", Message: "content is empty"}
	}
	if len(content) > maxArg || len(key) > maxArg {
		return &Error{Class: ClassRejected, Command: "remember", Message: fmt.Sprintf("memory is longer than the command line allows (%d KB)", maxArg>>10)}
	}
	res, a, cmd, err := c.memoryWrite(ctx, "remember", "--json", flag("key", key), "--", content)
	if err != nil {
		return err
	}
	if (a.Action != "remembered" && a.Action != "updated") || a.Key != key {
		return memoryRejected(cmd, res, "bd did not confirm the memory was stored")
	}
	return nil
}

// Forget implements [Client]. A key bd does not hold gives [ErrMemoryGone].
func (c *ExecClient) Forget(ctx context.Context, key string) error {
	if err := CheckMemoryKey(key); err != nil {
		return &Error{Class: ClassRejected, Command: "forget", Message: err.Error()}
	}
	res, a, cmd, err := c.memoryWrite(ctx, "forget", "--json", "--", key)
	if err != nil {
		return err
	}
	switch {
	case a.Found == "false":
		return ErrMemoryGone
	case a.Deleted != "true" || a.Key != key:
		return memoryRejected(cmd, res, "bd did not confirm the memory was forgotten")
	}
	return nil
}

// ConfigSet implements [Client]; bdash uses it only for the events-journal
// opt-in.
func (c *ExecClient) ConfigSet(ctx context.Context, key, value string) error {
	if err := checkID(key); err != nil {
		return err
	}
	res, r, cmd, err := c.write(ctx, "config", "set", key, value, "--json")
	if err != nil {
		return err
	}
	if err := c.judge(cmd, res, r, nil); err != nil {
		return err
	}
	var a struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if data, err := unwrap(cmd, res.Stdout); err == nil {
		_ = json.Unmarshal(data, &a)
	}
	if a.Key != key || a.Value != value {
		return memoryRejected(cmd, res, "bd did not confirm the setting")
	}
	return nil
}
