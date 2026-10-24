package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

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
	accent := fg(art.RGB{255, 255, 255})
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

func (m *Model) emptyTick() tea.Cmd {
	if m.section != secZilkroad || len(m.results) > 0 {
		return nil
	}
	return m.startPageTick()
}

func (m *Model) emptyLines(w, h int) []line {
	msg, style := "Nothing matches.", fg(m.theme.Dim)
	if len(m.q.Errs) > 0 {
		msg, style = m.q.Errs[0], fg(m.theme.Warn)
	}
	esc := "to clear or"
	if m.focus == focusQuery {
		esc = "to cancel or"
	}
	var keys row
	k := m.pageKey("esc", esc)
	keys.cells(k.s, k.w)
	keys.space(1)
	k = m.pageKey("?", "for help")
	keys.cells(k.s, k.w)
	var block []line
	if h >= art.ThumbH+5 {
		figure, _ := m.shedding().art(mix(m.theme.Dim, m.theme.BodyBg, 0.5))
		for _, l := range art.Thumb(figure, m.opts.Glyph) {
			block = append(block, centreIn(line{l, art.ThumbW}, w))
		}
		block = append(block, line{})
	}
	block = append(block, centreIn(text(style, clip(msg, w)), w), line{}, centreIn(keys.line(), w))
	return centre(block, 0, w, h)
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
