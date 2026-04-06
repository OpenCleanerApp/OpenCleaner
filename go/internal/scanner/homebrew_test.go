package scanner

import "testing"

func TestHomebrewScannerID(t *testing.T) {
	s := NewHomebrewScanner("/home")
	if s.ID() != "homebrew" {
		t.Errorf("expected 'homebrew', got %q", s.ID())
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		n        int
		singular string
		want     string
	}{
		{1, "item", "1 item"},
		{5, "item", "5 items"},
		{0, "file", "0 files"},
	}
	for _, tt := range tests {
		got := formatCount(tt.n, tt.singular)
		if got != tt.want {
			t.Errorf("formatCount(%d, %q) = %q, want %q", tt.n, tt.singular, got, tt.want)
		}
	}
}
