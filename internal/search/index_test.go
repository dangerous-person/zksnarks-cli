package search

import "testing"

func TestPrice(t *testing.T) {
	cases := []struct {
		s    string
		want float64
		ok   bool
	}{
		{"NaN", 0, false},
		{"nan", 0, false},
		{"Infinity", 0, false},
		{"-Infinity", 0, false},
		{"+Inf", 0, false},
		{"1e400", 0, false},
		{"-1e400", 0, false},
		{"", 0, false},
		{"abc", 0, false},
		{"12.5", 12.5, true},
		{"-3", -3, true},
		{"0.419", 0.419, true},
	}
	for _, c := range cases {
		if got, ok := Price(c.s); got != c.want || ok != c.ok {
			t.Errorf("Price(%q) = %v, %v, want %v, %v", c.s, got, ok, c.want, c.ok)
		}
	}
}
