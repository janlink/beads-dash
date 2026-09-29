package rows

import "testing"

func TestMidCut(t *testing.T) {
	if got := MidCut("short", 10, "…"); got != "short" {
		t.Errorf("short: %q", got)
	}
	if got := MidCut("abcdefghijklmnop", 9, "…"); got != "abcd…mnop" {
		t.Errorf("long: %q", got)
	}
	if got := MidCut("abcdefghijklmnop", 9, "..."); got != "abc...nop" {
		t.Errorf("ascii ellipsis: %q", got)
	}
}
