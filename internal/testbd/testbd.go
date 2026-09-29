// Package testbd runs tests against real bd binaries. TestMain seeds one
// template workspace per bd version and recipe; each test gets its own copy.
//
// Copies of one template share its bd project_id, so tests must not assume
// distinct ids across copies. Run makes the process hermetic: HOME and the
// XDG, Dolt and git identity settings point into a temporary directory, and
// BEADS_* and BD_* variables are removed.
package testbd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Versions lists every bd version with committed fixtures.
var Versions = []string{"1.2.2", "1.3.0"}

// DefaultName is the recipe name used by [New].
const DefaultName = "default"

const (
	envDir     = "BDASH_TEST_BD_DIR"
	envRequire = "BDASH_TEST_REQUIRE_BD"
	cmdTimeout = 2 * time.Minute
)

// Env is what a Recipe gets to seed a template workspace.
type Env struct {
	Bin     string
	Dir     string
	Version string
}

// Bd runs the bd binary in the workspace directory and returns stdout.
func (e Env) Bd(args ...string) (string, error) {
	return runIn(e.Bin, e.Dir, args...)
}

// Recipe seeds a freshly initialised workspace.
type Recipe func(Env) error

// Options configures Run.
type Options struct {
	// Versions defaults to [Versions].
	Versions []string
	// Recipes maps recipe names to seeders; each is seeded once per version.
	// Defaults to {DefaultName: DefaultRecipe}.
	Recipes map[string]Recipe
}

// DefaultRecipe creates a small epic with one child.
func DefaultRecipe(e Env) error {
	epic, err := e.Bd("create", "--silent", "--title=Epic", "--type=epic")
	if err != nil {
		return err
	}
	_, err = e.Bd("create", "--silent", "--title=Child", "--type=task", "--parent="+strings.TrimSpace(epic))
	return err
}

// Workspace is one test's private copy of a template workspace.
type Workspace struct {
	Bin     string
	Dir     string
	Version string
}

// Bd runs bd in the workspace and returns stdout.
func (w Workspace) Bd(args ...string) (string, error) {
	return runIn(w.Bin, w.Dir, args...)
}

// Cmd returns a bd command rooted in the workspace with the hermetic process
// environment. The caller may set Env, Stdin and the output writers.
func (w Workspace) Cmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, w.Bin, args...)
	cmd.Dir = w.Dir
	cmd.Env = os.Environ()
	return cmd
}

type key struct{ version, name string }

var (
	mu        sync.Mutex
	templates = map[key]Env{}
	missing   = map[string]string{}
	active    = Versions
)

// Run seeds the template workspaces, runs the tests and cleans up. Call it
// from TestMain: os.Exit(testbd.Run(m, testbd.Options{})). Nothing is seeded
// with -short.
func Run(m *testing.M, opts Options) int {
	if !flag.Parsed() {
		flag.Parse()
	}
	if testing.Short() {
		return m.Run()
	}
	if opts.Versions == nil {
		opts.Versions = Versions
	}
	if opts.Recipes == nil {
		opts.Recipes = map[string]Recipe{DefaultName: DefaultRecipe}
	}
	active = opts.Versions
	root, err := os.MkdirTemp("", "bdash-template-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testbd:", err)
		return 1
	}
	defer os.RemoveAll(root)
	if err := isolateEnv(filepath.Join(root, "home")); err != nil {
		fmt.Fprintln(os.Stderr, "testbd:", err)
		return 1
	}

	for _, v := range opts.Versions {
		bin, err := findBinary(v)
		if err != nil {
			mu.Lock()
			missing[v] = err.Error()
			mu.Unlock()
			continue
		}
		for name, recipe := range opts.Recipes {
			env := Env{Bin: bin, Dir: filepath.Join(root, "tpl", v, name), Version: v}
			if err := seed(env, recipe); err != nil {
				fmt.Fprintf(os.Stderr, "testbd: seeding bd %s recipe %s: %v\n", v, name, err)
				return 1
			}
			mu.Lock()
			templates[key{v, name}] = env
			mu.Unlock()
		}
	}
	return m.Run()
}

// isolateEnv points every per-user location into home and pins a git identity.
func isolateEnv(home string) error {
	if err := os.MkdirAll(home, 0o750); err != nil {
		return err
	}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BEADS_") || strings.HasPrefix(name, "BD_") {
			if err := os.Unsetenv(name); err != nil {
				return err
			}
		}
	}
	set := map[string]string{
		"HOME":                home,
		"USERPROFILE":         home,
		"XDG_CONFIG_HOME":     filepath.Join(home, ".config"),
		"XDG_DATA_HOME":       filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME":      filepath.Join(home, ".cache"),
		"DOLT_ROOT_PATH":      home,
		"GIT_CONFIG_GLOBAL":   filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_SYSTEM":   os.DevNull,
		"GIT_AUTHOR_NAME":     "bdash test",
		"GIT_AUTHOR_EMAIL":    "test@example.com",
		"GIT_COMMITTER_NAME":  "bdash test",
		"GIT_COMMITTER_EMAIL": "test@example.com",
	}
	for k, v := range set {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// required reports whether BDASH_TEST_REQUIRE_BD demands this version: "1" or
// "all" for every version, otherwise a comma-separated list.
func required(version string) bool {
	v := os.Getenv(envRequire)
	if v == "1" || v == "all" {
		return true
	}
	for _, r := range strings.Split(v, ",") {
		if strings.TrimSpace(r) == version {
			return true
		}
	}
	return false
}

// New copies the default template workspace for version into t.TempDir().
func New(t testing.TB, version string) Workspace {
	t.Helper()
	return NewNamed(t, version, DefaultName)
}

// NewNamed copies the template seeded by the named recipe. It skips the test
// with -short, and when the bd binary is unavailable unless
// BDASH_TEST_REQUIRE_BD demands that version, in which case it fails.
func NewNamed(t testing.TB, version, name string) Workspace {
	t.Helper()
	if testing.Short() {
		t.Skip("real-bd integration test skipped with -short")
	}
	mu.Lock()
	tpl, ok := templates[key{version, name}]
	why := missing[version]
	mu.Unlock()
	if !ok {
		if why == "" {
			why = fmt.Sprintf("template %q not seeded: TestMain must call testbd.Run with it", name)
		}
		if required(version) {
			t.Fatalf("bd %s unavailable: %s", version, why)
		}
		t.Skipf("bd %s unavailable: %s", version, why)
	}
	dir := t.TempDir()
	if err := copyTree(tpl.Dir, dir); err != nil {
		t.Fatalf("copy template workspace: %v", err)
	}
	return Workspace{Bin: tpl.Bin, Dir: dir, Version: version}
}

// EachVersion runs f as a subtest for every version given to Run.
func EachVersion(t *testing.T, f func(t *testing.T, w Workspace)) {
	t.Helper()
	for _, v := range active {
		t.Run("bd-"+v, func(t *testing.T) {
			f(t, New(t, v))
		})
	}
}

// findBinary resolves the bd binary for a version: BDASH_TEST_BD_<ver> (dots
// as underscores), then <BDASH_TEST_BD_DIR>/<ver>/bd, then
// <repo>/.cache/bd/<ver>/bd as written by scripts/install-bd.sh.
func findBinary(version string) (string, error) {
	name := "bd"
	if runtime.GOOS == "windows" {
		name = "bd.exe"
	}
	if p := os.Getenv("BDASH_TEST_BD_" + strings.ReplaceAll(version, ".", "_")); p != "" {
		return checkBinary(p, version)
	}
	dirs := []string{}
	if d := os.Getenv(envDir); d != "" {
		dirs = append(dirs, d)
	}
	if root, err := repoRoot(); err == nil {
		dirs = append(dirs, filepath.Join(root, ".cache", "bd"))
	}
	for _, d := range dirs {
		p := filepath.Join(d, version, name)
		if _, err := os.Stat(p); err == nil {
			return checkBinary(p, version)
		}
	}
	return "", fmt.Errorf("no binary (set BDASH_TEST_BD_%s or %s, or run scripts/install-bd.sh %s)",
		strings.ReplaceAll(version, ".", "_"), envDir, version)
}

func checkBinary(path, version string) (string, error) {
	out, err := runIn(path, "", "version")
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if !strings.Contains(out, "bd version "+version+" ") {
		return "", fmt.Errorf("%s reports %q, want version %s", path, strings.TrimSpace(out), version)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

func seed(e Env, recipe Recipe) error {
	if err := os.MkdirAll(e.Dir, 0o750); err != nil {
		return err
	}
	if _, err := runIn("git", e.Dir, "init", "-q"); err != nil {
		return err
	}
	if _, err := e.Bd("init", "--non-interactive", "--prefix", "t", "--stealth"); err != nil {
		return err
	}
	return recipe(e)
}

func runIn(bin, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", filepath.Base(bin), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return nil
		}
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
