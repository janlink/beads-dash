package bd

import (
	"context"
	"testing"
)

func TestParseVersion(t *testing.T) {
	ok := map[string]Version{
		"1.2.2": {1, 2, 2}, "v1.3.0": {1, 3, 0}, "1.3.1-rc.1": {1, 3, 1}, "2.0.0": {2, 0, 0}, "1.4.0+build": {1, 4, 0},
	}
	for in, want := range ok {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1.2", "1.2.x", "a.b.c", "1.2.3.4", "-1.2.3", "HEAD"} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) succeeded", in)
		}
	}
	if (Version{1, 2, 3}).String() != "1.2.3" {
		t.Error("String")
	}
}

func TestVersionCompare(t *testing.T) {
	a, b := Version{1, 2, 2}, Version{1, 3, 0}
	if a.Compare(b) != -1 || b.Compare(a) != 1 || a.Compare(Version{1, 2, 2}) != 0 {
		t.Error("Compare")
	}
	if (Version{2, 0, 0}).Compare(Version{1, 9, 9}) != 1 || (Version{1, 2, 3}).Compare(Version{1, 2, 2}) != 1 {
		t.Error("Compare major/patch")
	}
}

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		raw     string
		support Support
		journal bool
		wantErr bool
	}{
		{"1.0.0", Unsupported, false, true},
		{"1.2.0", Unsupported, false, true},
		{"1.2.1", Unsupported, false, true},
		{"1.2.2", Supported, false, false},
		{"1.2.2-rc.1", Unsupported, false, true},
		{"v1.2.2-rc.2", Unsupported, false, true},
		{"1.2.9", Supported, false, false},
		{"1.3.0", Supported, true, false},
		{"1.3.1-rc.1", Supported, true, false},
		{"1.3.7", Supported, true, false},
		{"1.4.0", Untested, true, false},
		{"2.0.0", Untested, true, false},
		{"garbage", Unsupported, false, true},
		{"", Unsupported, false, true},
	}
	for _, tc := range tests {
		info, err := CheckVersion(tc.raw)
		if info.Support != tc.support || info.caps.eventsJournal != tc.journal || (err != nil) != tc.wantErr {
			t.Errorf("CheckVersion(%q) = %+v, %v", tc.raw, info, err)
		}
		if tc.wantErr && !IsClass(err, ClassUnsupported) {
			t.Errorf("CheckVersion(%q) error class = %v", tc.raw, err)
		}
		if tc.support != Supported && info.Reason == "" {
			t.Errorf("CheckVersion(%q) has no reason", tc.raw)
		}
	}
}

func TestUntestedUsesCapabilitiesOfHighestTested(t *testing.T) {
	newer, _ := CheckVersion("1.9.0")
	tested, _ := CheckVersion("1.3.0")
	if newer.caps != tested.caps {
		t.Errorf("untested caps = %+v, want %+v", newer.caps, tested.caps)
	}
	if old, _ := CheckVersion("1.2.2"); old.caps.eventsJournal {
		t.Errorf("1.2.2 caps = %+v", old.caps)
	}
}

func TestSupportString(t *testing.T) {
	for s, want := range map[Support]string{Supported: "supported", Untested: "untested", Unsupported: "unsupported", Support(9): "unknown"} {
		if s.String() != want {
			t.Errorf("%d = %q", s, s.String())
		}
	}
}

func TestVersionCallReportsUnsupportedWithInfo(t *testing.T) {
	c := NewExec(ExecOptions{Runner: replay{"version": {Stdout: []byte(`{"schema_version":1,"data":{"version":"1.2.1","build":"b","commit":"c","branch":"v1.2.1"}}`)}}})
	info, err := c.Version(context.Background())
	if !IsClass(err, ClassUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if info.Raw != "1.2.1" || info.Build != "b" || info.Commit != "c" || info.Support != Unsupported {
		t.Errorf("info = %+v", info)
	}
	c = NewExec(ExecOptions{Runner: replay{"version": {Stdout: []byte(`{"schema_version":1,"data":[1]}`)}}})
	if _, err := c.Version(context.Background()); !IsClass(err, ClassTransient) {
		t.Errorf("wrong shape = %v", err)
	}
}

func TestProbeMissingBinary(t *testing.T) {
	_, err := Probe(context.Background(), "/nonexistent/dir/bd", Timeouts{})
	if !IsClass(err, ClassBdMissing) {
		t.Errorf("Probe of a missing binary = %v", err)
	}
}
