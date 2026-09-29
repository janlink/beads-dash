package rows

import "testing"

func TestOneLineReplacesC0AndC1Controls(t *testing.T) {
	got := oneLine("a\tb\x1bc\u0085d\u009be\u009ff\u00a0g\x7fh")
	if want := "a b c d e f\u00a0g h"; got != want {
		t.Errorf("oneLine = %q, want %q", got, want)
	}
}
