package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/testbd"
)

const treeWorkspace = "tree"

func TestMain(m *testing.M) {
	os.Exit(testbd.Run(m, testbd.Options{Recipes: map[string]testbd.Recipe{treeWorkspace: seedTree, testbd.DefaultName: testbd.DefaultRecipe}}))
}

func seedTree(e testbd.Env) error {
	create := func(args ...string) (string, error) {
		out, err := e.Bd(append([]string{"create", "--silent"}, args...)...)
		return strings.TrimSpace(out), err
	}
	epic, err := create("--title=Checkout epic", "--type=epic", "--priority=1")
	if err != nil {
		return err
	}
	open, err := create("--title=Address form", "--type=task", "--parent="+epic)
	if err != nil {
		return err
	}
	done, err := create("--title=Order summary", "--type=task", "--parent="+epic)
	if err != nil {
		return err
	}
	waiting, err := create("--title=Confirmation mail", "--type=task", "--parent="+epic)
	if err != nil {
		return err
	}
	if _, err := create("--title=Payment retries", "--type=bug", "--priority=0", "--assignee=alice"); err != nil {
		return err
	}
	for _, args := range [][]string{{"close", done}, {"dep", "add", waiting, open}} {
		if _, err := e.Bd(args...); err != nil {
			return err
		}
	}
	return nil
}

func TestIntegrationTreeAndReadyRenderFromARealWorkspace(t *testing.T) {
	for _, v := range testbd.Versions {
		t.Run("bd-"+v, func(t *testing.T) {
			w := testbd.NewNamed(t, v, treeWorkspace)
			a := New(testOptions(plain, func(o *Options) {
				withView("tree", false)(o)
				o.Client = bd.NewExec(bd.ExecOptions{Bin: w.Bin, Dir: w.Dir})
				o.Now = time.Now
			}))
			send(a, tea.WindowSizeMsg{Width: 120, Height: 30})
			cmd := send(a, a.Init()())
			for range 5 {
				if cmd == nil || a.snap != nil {
					break
				}
				cmd = send(a, cmd())
			}
			defer func() {
				if a.eng != nil {
					a.eng.Stop()
					a.cancel()
				}
			}()
			if a.snap == nil {
				t.Fatalf("no snapshot:\n%s", screen(a))
			}
			out := screen(a)
			for _, want := range []string{"Checkout epic", "Address form", "Confirmation mail", "Payment retries", "1 closed"} {
				if !strings.Contains(out, want) {
					t.Errorf("tree lacks %q:\n%s", want, out)
				}
			}

			press(a, "4")
			out = screen(a)
			for _, want := range []string{"Unassigned ", "Assigned, not started 1", "Payment retries", "Address form", "Blocked 1", "1 container hidden"} {
				if !strings.Contains(out, want) {
					t.Errorf("ready lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "Checkout epic") {
				t.Errorf("ready lists the container:\n%s", out)
			}
		})
	}
}
