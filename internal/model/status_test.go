package model_test

import (
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func customStatuses() model.Statuses {
	return model.NewStatuses(append(model.BuiltinStatuses().All(),
		model.StatusInfo{Name: "review", Category: model.CategoryWIP, Custom: true},
		model.StatusInfo{Name: "later", Category: model.CategoryFrozen, Custom: true},
		model.StatusInfo{Name: "triage", Category: model.CategoryActive, Custom: true},
		model.StatusInfo{Name: "shipped", Category: model.CategoryDone, Custom: true},
		model.StatusInfo{Name: "plain", Category: model.CategoryUnspecified, Custom: true},
	))
}

func TestPresent(t *testing.T) {
	tests := []struct {
		raw     string
		blocked bool
		want    model.Presentation
	}{
		{"open", false, model.Presentation{Status: model.Open}},
		{"open", true, model.Presentation{Status: model.Blocked}},
		{"blocked", false, model.Presentation{Status: model.Blocked}},
		{"blocked", true, model.Presentation{Status: model.Blocked}},
		{"in_progress", false, model.Presentation{Status: model.InProgress}},
		{"in_progress", true, model.Presentation{Status: model.InProgress, BlockedMarker: true}},
		{"hooked", false, model.Presentation{Status: model.InProgress}},
		{"hooked", true, model.Presentation{Status: model.InProgress, BlockedMarker: true}},
		{"deferred", false, model.Presentation{Status: model.Frozen}},
		{"deferred", true, model.Presentation{Status: model.Frozen}},
		{"pinned", false, model.Presentation{Status: model.Frozen}},
		{"closed", false, model.Presentation{Status: model.Closed}},
		{"closed", true, model.Presentation{Status: model.Closed}},
		{"triage", false, model.Presentation{Status: model.Open}},
		{"triage", true, model.Presentation{Status: model.Blocked}},
		{"review", false, model.Presentation{Status: model.InProgress}},
		{"review", true, model.Presentation{Status: model.InProgress, BlockedMarker: true}},
		{"later", false, model.Presentation{Status: model.Frozen}},
		{"shipped", false, model.Presentation{Status: model.Closed}},
		{"plain", false, model.Presentation{Status: model.Other}},
		{"plain", true, model.Presentation{Status: model.Other}},
		{"parked", false, model.Presentation{Status: model.Other}},
		{"parked", true, model.Presentation{Status: model.Other}},
		{"", false, model.Presentation{Status: model.Other}},
	}
	st := customStatuses()
	for _, tc := range tests {
		if got := model.Present(tc.raw, tc.blocked, st); got != tc.want {
			t.Errorf("Present(%q, blocked=%v) = %+v, want %+v", tc.raw, tc.blocked, got, tc.want)
		}
	}
}

func TestZeroStatusesAreBuiltin(t *testing.T) {
	var st model.Statuses
	if got := st.Category("closed"); got != model.CategoryDone {
		t.Errorf("Category(closed) = %q", got)
	}
	if got := model.Present("review", false, st); got.Status != model.Other {
		t.Errorf("custom status without table = %v, want Other", got.Status)
	}
	if n := len(st.All()); n != 7 {
		t.Errorf("All() = %d statuses, want 7", n)
	}
}

func TestStatusesLookupAndNames(t *testing.T) {
	st := customStatuses()
	in, ok := st.Lookup("review")
	if !ok || !in.Custom || in.Category != model.CategoryWIP {
		t.Errorf("Lookup(review) = %+v, %v", in, ok)
	}
	if _, ok := st.Lookup("nope"); ok {
		t.Error("Lookup(nope) found")
	}
	names := st.Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Names not sorted: %v", names)
		}
	}
	dup := model.NewStatuses([]model.StatusInfo{
		{Name: "a", Category: model.CategoryActive},
		{Name: "a", Category: model.CategoryDone},
	})
	if len(dup.All()) != 1 || dup.Category("a") != model.CategoryDone {
		t.Errorf("duplicate names: %+v", dup.All())
	}
}

func TestParseCategory(t *testing.T) {
	for in, want := range map[string]model.Category{
		"active": model.CategoryActive, "wip": model.CategoryWIP, "done": model.CategoryDone,
		"frozen": model.CategoryFrozen, "unspecified": model.CategoryUnspecified,
		"": model.CategoryUnknown, "nonsense": model.CategoryUnknown,
	} {
		if got := model.ParseCategory(in); got != want {
			t.Errorf("ParseCategory(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPresentationStatusString(t *testing.T) {
	want := map[model.PresentationStatus]string{
		model.Open: "Open", model.InProgress: "In progress", model.Blocked: "Blocked",
		model.Frozen: "Frozen", model.Closed: "Closed", model.Other: "Other",
	}
	for p, s := range want {
		if p.String() != s {
			t.Errorf("%d.String() = %q, want %q", p, p.String(), s)
		}
	}
}
