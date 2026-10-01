package detail

import "testing"

func TestPlace(t *testing.T) {
	tests := []struct {
		name       string
		cols, body int
		board      bool
		want       Dock
	}{
		{"side at the split width", 107, 30, false, Dock{Side, 37, 30}},
		{"side grows to its widest", 111, 30, false, Dock{Side, 41, 30}},
		{"side stays at its widest", 200, 48, false, Dock{Side, 41, 48}},
		{"short terminals still split", 200, 8, false, Dock{Side, 41, 8}},
		{"overlay below the split width", 106, 30, false, Dock{Overlay, 106, 30}},
		{"board splits from two columns and the panel", 90, 30, true, Dock{Side, 41, 30}},
		{"board overlays below that", 89, 30, true, Dock{Overlay, 89, 30}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Place(tt.cols, tt.body, tt.board); got != tt.want {
				t.Errorf("Place(%d, %d, %v) = %+v, want %+v", tt.cols, tt.body, tt.board, got, tt.want)
			}
		})
	}
}
