// Package testgolden compares rendered output with golden files under the
// calling package's testdata directory. Run tests with -update to rewrite them.
package testgolden

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// Equal asserts that got matches testdata/<test name>.golden. Control codes
// are escaped at compare time only; golden files hold the raw output.
func Equal(t testing.TB, got string) {
	t.Helper()
	golden.RequireEqual(t, got)
}
