package testbd_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/testbd"
)

func TestMain(m *testing.M) {
	os.Exit(testbd.Run(m, testbd.Options{Recipes: map[string]testbd.Recipe{
		testbd.DefaultName: testbd.DefaultRecipe,
		"empty":            func(testbd.Env) error { return nil },
	}}))
}

func TestIntegrationVersionJSON(t *testing.T) {
	testbd.EachVersion(t, func(t *testing.T, w testbd.Workspace) {
		out, err := w.Bd("version", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("decode %q: %v", out, err)
		}
		if v.Version != w.Version {
			t.Errorf("version = %q, want %q", v.Version, w.Version)
		}
	})
}

func TestIntegrationWorkspacesAreIsolated(t *testing.T) {
	a := testbd.New(t, testbd.Versions[0])
	b := testbd.New(t, testbd.Versions[0])
	if _, err := a.Bd("create", "--silent", "--title=only in a"); err != nil {
		t.Fatal(err)
	}
	count := func(w testbd.Workspace) int {
		out, err := w.Bd("list", "--json", "--all")
		if err != nil {
			t.Fatal(err)
		}
		var l []json.RawMessage
		if err := json.Unmarshal([]byte(out), &l); err != nil {
			t.Fatal(err)
		}
		return len(l)
	}
	if count(a) != count(b)+1 {
		t.Errorf("workspaces share state: a=%d b=%d", count(a), count(b))
	}
}

func TestIntegrationNamedRecipe(t *testing.T) {
	list := func(w testbd.Workspace) []json.RawMessage {
		out, err := w.Bd("list", "--json", "--all")
		if err != nil {
			t.Fatal(err)
		}
		var l []json.RawMessage
		if err := json.Unmarshal([]byte(out), &l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	if n := len(list(testbd.NewNamed(t, testbd.Versions[0], "empty"))); n != 0 {
		t.Errorf("empty recipe has %d issues", n)
	}
	if n := len(list(testbd.New(t, testbd.Versions[0]))); n != 2 {
		t.Errorf("default recipe has %d issues, want 2", n)
	}
}

func TestIntegrationCmdIsHermetic(t *testing.T) {
	w := testbd.New(t, testbd.Versions[0])
	if home := os.Getenv("HOME"); !strings.Contains(filepath.Base(filepath.Dir(home)), "bdash-template-") {
		t.Errorf("HOME = %q, want a bdash-template temp dir", home)
	}
	cmd := w.Cmd(context.Background(), "version", "--json")
	cmd.Env = append(cmd.Env, "EXTRA=1")
	if out, err := cmd.Output(); err != nil || !strings.Contains(string(out), w.Version) {
		t.Errorf("Cmd output %q, err %v", out, err)
	}
}
