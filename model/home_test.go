package model

import "testing"

func TestNormalizeVNPhone(t *testing.T) {
	tests := map[string]string{
		"0900000001":   "84900000001",
		"+84900000001": "84900000001",
		"84900000001":  "84900000001",
		"090 000 0001": "84900000001",
	}
	for in, want := range tests {
		if got := NormalizeVNPhone(in); got != want {
			t.Errorf("NormalizeVNPhone(%q) = %q, want %q", in, got, want)
		}
	}
}
