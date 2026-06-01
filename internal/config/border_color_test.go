package config

import "testing"

func TestParseBorderColor(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"#FF3344", "#FF3344", false},
		{"#ff3344", "#ff3344", false},
		{"#F33", "#F33", false},
		{"#f33", "#f33", false},
		{"red", "", true},
		{"#FF334", "", true},   // 6 chars total — wrong length
		{"#FF33445", "", true}, // 8 chars total — wrong length
		{"#GGGGGG", "", true},  // non-hex
		{"FF3344", "", true},   // missing #
		{"#12 345", "", true},  // space in hex
	}
	for _, tc := range cases {
		got, err := parseBorderColor(tc.in, nil)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseBorderColor(%q) want error, got nil", tc.in)
			}
			if got != "" {
				t.Errorf("parseBorderColor(%q) on error want \"\", got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseBorderColor(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseBorderColor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
