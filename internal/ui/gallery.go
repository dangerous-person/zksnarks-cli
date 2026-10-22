package ui

import (
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

// galleryLines draws the visible part of the thumbnail grid. The grid is one long strip of
// rows, each a gap line (which carries the selection's corner marks), 13 lines of art and a
// label; only the lines inside the viewport are built.
func (m *Model) galleryLines(w, h int) []line {
	out := make([]line, h)
	n := len(m.results)
	if n == 0 {
		return m.emptyLines(w, h)
	}
	cols := m.cols()
	gridW := cols*slotW + (cols-1)*slotGap
	padL := max(0, (w-gridW)/2)
	accent := fg(m.theme.Selection)
	total := m.contentH()
	for y := 0; y < h; y++ {
		ln := m.scroll + y
		if ln >= total {
			break
		}
		r, k := ln/pitch, ln%pitch
		var b row
		b.space(padL)
		for c := 0; c < cols; c++ {
			if c > 0 {
				b.space(slotGap)
			}
			idx := r*cols + c
			switch {
			case k == 0:
				switch {
				case idx == m.sel:
					b.put(accent, "╭─")
					b.space(slotW - 4)
					b.put(accent, "─╮")
				case r > 0 && idx-cols == m.sel:
					b.put(accent, "╰─")
					b.space(slotW - 4)
					b.put(accent, "─╯")
				default:
					b.space(slotW)
				}
			case idx >= n:
				b.space(slotW)
			case k <= art.ThumbH:
				b.space(1)
				b.cells(m.thumb(m.results[idx])[k-1], art.ThumbW)
				b.space(1)
			default:
				b.space(1)
				l := m.label(&m.tokens[m.results[idx]], idx == m.sel, art.ThumbW)
				b.cells(l.s, l.w)
				b.space(1)
			}
		}
		out[y] = b.line()
	}
	return out
}

// label is "#N" on the left and the price (or status) on the right.
func (m *Model) label(t *atlas.Token, sel bool, w int) line {
	idStyle := fg(m.theme.Text)
	if sel {
		idStyle = bold(m.theme.Selection)
	}
	price, pStyle := "-", fg(m.theme.Dim)
	switch {
	case t.Purged:
		price, pStyle = "purged", fg(m.theme.Warn)
	case t.Listed && t.Ask != "":
		price, pStyle = zec(t.Ask)+" ZEC", fg(m.theme.Price)
	}
	return leftRight(idStyle, "#"+itoa(t.ID), pStyle, price, w)
}

func (m *Model) emptyLines(w, h int) []line {
	out := make([]line, h)
	msg := "no tokens match"
	if len(m.q.Errs) > 0 {
		msg = m.q.Errs[0]
	}
	y := min(h-1, 3)
	pad := max(0, (w-width(msg))/2)
	var r row
	r.space(pad)
	r.put(fg(m.theme.Dim), clip(msg, w))
	out[y] = r.line()
	if y+2 < h {
		hint := "esc clears the query"
		var r2 row
		r2.space(max(0, (w-width(hint))/2))
		r2.put(fg(m.theme.Dim), hint)
		out[y+2] = r2.line()
	}
	return out
}

// swatch is the most common non-black colour of a token, for the list view.
func swatch(a art.Art) art.RGB {
	counts := make([]int, len(a.Palette))
	for _, p := range a.Pix {
		counts[p]++
	}
	best, bestN := art.RGB{}, -1
	for i, n := range counts {
		if c := a.Palette[i]; c != (art.RGB{}) && n > bestN {
			best, bestN = c, n
		}
	}
	return best
}

func bgCell(c art.RGB, s string) string {
	return strings.Replace(fg(c), "38;", "48;", 1) + s + reset
}
