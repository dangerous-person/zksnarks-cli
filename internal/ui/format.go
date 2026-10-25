package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func itoa(n int) string { return strconv.Itoa(n) }

// comma writes an integer with thousands separators.
func comma(n int) string {
	s, sign := strconv.FormatInt(int64(n), 10), ""
	if s[0] == '-' {
		s, sign = s[1:], "-"
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

func roundInt(v float64) (int, bool) {
	if math.IsNaN(v) || math.Abs(v) > 1e15 {
		return 0, false
	}
	return int(math.Round(v)), true
}

// zec formats an amount the way the API wrote it, with at least two decimals.
func zec(s string) string {
	if s == "" {
		return "-"
	}
	if i := strings.IndexByte(s, '.'); i < 0 {
		return s + ".00"
	} else if len(s)-i-1 == 1 {
		return s + "0"
	}
	return s
}

func zecFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return zec(s)
}

func usd(f float64) string {
	n, ok := roundInt(f)
	if !ok {
		return ""
	}
	return "$" + comma(n)
}

func percent(n, total int) string {
	if total == 0 {
		return "-"
	}
	p := float64(n) * 100 / float64(total)
	if p >= 10 {
		return fmt.Sprintf("%.0f%%", p)
	}
	return fmt.Sprintf("%.1f%%", p)
}

// ago writes a duration since t in a few characters.
func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}
