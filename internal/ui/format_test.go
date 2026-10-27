package ui

import (
	"math"
	"testing"

	"github.com/dangerous-person/zksnarks-cli/internal/market"
)

func TestComma(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{-7, "-7"},
		{999, "999"},
		{-999, "-999"},
		{1000, "1,000"},
		{-1000, "-1,000"},
		{1234567, "1,234,567"},
		{math.MinInt64, "-9,223,372,036,854,775,808"},
		{math.MaxInt64, "9,223,372,036,854,775,807"},
	}
	for _, c := range cases {
		if got := comma(c.n); got != c.want {
			t.Errorf("comma(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestRoundInt(t *testing.T) {
	cases := []struct {
		v    float64
		want int
		ok   bool
	}{
		{0, 0, true},
		{2.4, 2, true},
		{2.5, 3, true},
		{-2.5, -3, true},
		{6627.73217299, 6628, true},
		{1e15, 1_000_000_000_000_000, true},
		{-1e15, -1_000_000_000_000_000, true},
		{1e15 + 1, 0, false},
		{-1e16, 0, false},
		{1e300, 0, false},
		{-1e300, 0, false},
		{math.NaN(), 0, false},
		{math.Inf(1), 0, false},
		{math.Inf(-1), 0, false},
	}
	for _, c := range cases {
		if got, ok := roundInt(c.v); got != c.want || ok != c.ok {
			t.Errorf("roundInt(%v) = %d, %v, want %d, %v", c.v, got, ok, c.want, c.ok)
		}
	}
}

func TestAmounts(t *testing.T) {
	cases := []struct {
		in, usd, volume, stat string
	}{
		{"6627.73217299", "≈ $6,628", "6,628 ZEC", "6,628 ZEC"},
		{"10.88", "≈ $11", "11 ZEC", "10.88 ZEC"},
		{"5.57278", "≈ $6", "6 ZEC", "5.573 ZEC"},
		{"NaN", "", "", ""},
		{"Infinity", "", "", ""},
		{"-Infinity", "", "", ""},
		{"1e300", "", "", ""},
		{"-1e300", "", "", ""},
		{"1e400", "", "", ""},
		{"", "", "", ""},
		{"abc", "", "", ""},
	}
	for _, c := range cases {
		m := &Model{stats: &market.Stats{VolumeTotal: market.Decimal(c.in)}, rate: &market.Rate{ZecUsd: 1}}
		if got := m.usdText(c.in); got != c.usd {
			t.Errorf("usdText(%q) = %q, want %q", c.in, got, c.usd)
		}
		if got := m.volumeText(); got != c.volume {
			t.Errorf("volumeText(%q) = %q, want %q", c.in, got, c.volume)
		}
		if got := statZec(c.in); got != c.stat {
			t.Errorf("statZec(%q) = %q, want %q", c.in, got, c.stat)
		}
	}
	for _, f := range []float64{1e300, -1e300, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := usd(f); got != "" {
			t.Errorf("usd(%v) = %q, want empty", f, got)
		}
		if got := trimFloat(f); got != "" {
			t.Errorf("trimFloat(%v) = %q, want empty", f, got)
		}
	}
}

func TestTruncateCutEscape(t *testing.T) {
	for _, s := range []string{"\x1b", "abc\x1b[", "abc\x1b[38;2;1;2"} {
		if got := truncate(line{s, 10}, 2); got.w > 2 {
			t.Errorf("truncate(%q) = %q, %d wide", s, got.s, got.w)
		}
	}
}
