package lintcheck_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/tools/lintcheck"
)

func check(t *testing.T, src, pkgPath string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return lintcheck.Check(fset, f, pkgPath)
}

func TestCheckFindings(t *testing.T) {
	tests := []struct {
		name, src, pkg string
		want           int
	}{
		{"non-first arg", `package p; var _ = filepath.Join(root, ".beads")`, "internal/ui", 1},
		{"const", `package p; const d = ".beads/"`, "internal/ui", 1},
		{"var", `package p; var d = "x/.beads/last-touched"`, "internal/ui", 1},
		{"concatenation", `package p; var d = root + "/.beads" + "/x"`, "internal/ui", 1},
		{"raw string", "package p; var d = `.beads`", "internal/ui", 1},
		{"show in bd", `package p; var _ = exec.Command("bd", "show", "x")`, "internal/bd", 1},
		{"show in bd subpackage", `package p; var a = []string{"show"}`, "internal/bd/parse", 1},
		{"show elsewhere", `package p; var a = []string{"show"}`, "internal/ui", 0},
		{"show substring", `package p; var a = "show me"`, "internal/bd", 0},
		{"allowed", "package p; var d = \".beads\" // lintcheck:allow user-facing text", "internal/ui", 0},
		{"clean", `package p; var d = "hello"`, "internal/ui", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := check(t, tt.src, tt.pkg); len(got) != tt.want {
				t.Errorf("findings = %v, want %d", got, tt.want)
			}
		})
	}
}

func TestRepositoryIsClean(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch {
			case d.Name() == ".git" || d.Name() == ".cache" || d.Name() == "testdata",
				rel == "tools", rel == "internal/testbd":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, finding := range lintcheck.Check(fset, f, filepath.ToSlash(filepath.Dir(rel))) {
			t.Error(finding)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
