package ui

import (
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

// word is one run of non-space text, possibly mixing highlighted and plain parts.
type word struct {
	parts []site.Span
	w     int
}

func words(spans []site.Span) []word {
	var out []word
	var cur word
	flush := func() {
		if cur.w > 0 {
			out = append(out, cur)
		}
		cur = word{}
	}
	for _, sp := range spans {
		for i, f := range strings.Split(sp.Text, " ") {
			if i > 0 {
				flush()
			}
			if f != "" {
				cur.parts = append(cur.parts, site.Span{Text: f, Hi: sp.Hi})
				cur.w += width(f)
			}
		}
	}
	flush()
	return out
}

// wrap breaks spans into lines of at most w cells.
func wrap(spans []site.Span, w int) [][]site.Span {
	var lines [][]site.Span
	var cur []site.Span
	cw := 0
	for _, wd := range words(spans) {
		if cw > 0 && cw+1+wd.w > w {
			lines = append(lines, cur)
			cur, cw = nil, 0
		}
		if cw > 0 {
			cur = append(cur, site.Span{Text: " ", Hi: cur[len(cur)-1].Hi && wd.parts[0].Hi})
			cw++
		}
		cur = append(cur, wd.parts...)
		cw += wd.w
	}
	if cw > 0 {
		lines = append(lines, cur)
	}
	return lines
}

func plain(s string) []site.Span { return []site.Span{{Text: s}} }

// spanLine draws at most limit characters of a wrapped line and reports how many it drew.
func spanLine(r *row, spans []site.Span, normal, hi string, limit int) int {
	n := 0
	for _, s := range spans {
		if n >= limit {
			break
		}
		t := s.Text
		if rest := limit - n; width(t) > rest {
			t = string([]rune(t)[:rest])
		}
		style := normal
		if s.Hi {
			style = hi
		}
		r.put(style, t)
		n += width(t)
	}
	return n
}

func (m *Model) spotlight() (site.Spotlight, bool) {
	s := m.opts.Site.Spotlights
	if len(s) == 0 {
		return site.Spotlight{}, false
	}
	return s[m.spot], true
}
