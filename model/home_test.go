package model

import "testing"

func TestNormalizeVNPhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0900000001", "84900000001"},
		{"+84900000001", "84900000001"},
		{"84900000001", "84900000001"},
		{"090 000 0001", "84900000001"},
		{"084900000001", "84900000001"},
		{"0084900000001", "84900000001"},
		{"00900000001", "84900000001"},
		{"+840900000001", "84900000001"},
		// 084x is a real Vinaphone prefix: the subscriber part IS 842345678, so
		// this must NOT collapse to 842345678.
		{"0842345678", "84842345678"},
		{"84842345678", "84842345678"},
		{"123", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeVNPhone(c.in); got != c.want {
			t.Errorf("NormalizeVNPhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
