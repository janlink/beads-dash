package detail

import "testing"

func TestPlace(t *testing.T) {
	tests := []struct {
		name       string
		cols, body int
		docked     bool
		want       Dock
	}{
		{"side at 200", 200, 48, true, Dock{Side, 90, 48}},
		{"side at exactly 20 rows", 200, 20, true, Dock{Side, 90, 20}},
		{"wide but short overlays", 200, 19, true, Dock{Frame: Hidden}},
		{"bottom at 199", 199, 48, true, Dock{Bottom, 199, 19}},
		{"bottom at 80x24", 80, 22, true, Dock{Bottom, 80, 10}},
		{"bottom height is 40 percent", 120, 38, true, Dock{Bottom, 120, 15}},
		{"bottom needs 20 rows", 120, 19, true, Dock{Frame: Hidden}},
		{"bottom at exactly 20 rows", 120, 20, true, Dock{Bottom, 120, 10}},
		{"narrow overlays", 79, 40, true, Dock{Frame: Hidden}},
		{"setting off", 200, 48, false, Dock{Frame: Hidden}},
		{"setting off, bottom", 100, 30, false, Dock{Frame: Hidden}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Place(tt.cols, tt.body, tt.docked); got != tt.want {
				t.Errorf("Place(%d, %d, %v) = %+v, want %+v", tt.cols, tt.body, tt.docked, got, tt.want)
			}
		})
	}
}
