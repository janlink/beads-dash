// Package lintcheck rejects string literals that golangci-lint cannot match:
// any literal containing ".beads" and, in internal/bd, the literal "show".
package lintcheck

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

const allowMarker = "lintcheck:allow"

// Check returns one finding per forbidden literal in file. pkgPath is the
// slash-separated directory of the file relative to the module root. A line
// carrying a "lintcheck:allow" comment is exempt.
func Check(fset *token.FileSet, file *ast.File, pkgPath string) []string {
	allowed := map[int]bool{}
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, allowMarker) {
				allowed[fset.Position(c.Pos()).Line] = true
			}
		}
	}
	inBd := pkgPath == "internal/bd" || strings.HasPrefix(pkgPath, "internal/bd/")
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pos := fset.Position(lit.Pos())
		if allowed[pos.Line] {
			return true
		}
		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		switch {
		case strings.Contains(val, ".beads"):
			out = append(out, pos.String()+": no direct .beads/ access, go through the bd CLI")
		case inBd && val == "show":
			out = append(out, pos.String()+": bd show writes .beads/last-touched and is never called; use list and comments")
		}
		return true
	})
	return out
}
