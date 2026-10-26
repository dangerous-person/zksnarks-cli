package ui

import (
	"slices"
	"sort"
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
	case t.Listed && priced(t.Ask):
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

// swatch is the colour of a token's skin, for the list view: its largest group of near
// shades among the colours that most hatless tokens of the same lineage and skin share and
// most other skins of that lineage do not, near-black groups counting for less. So neither a
// hat nor a lineage's own dark parts win over a skin drawn in several shades.
func (m *Model) swatch(i int) art.RGB {
	if c, ok := m.swatches[i]; ok {
		return c
	}
	if m.skins == nil {
		m.skins = skinColours(m.tokens)
	}
	t := &m.tokens[i]
	shared := m.skins[[2]string{t.Traits[0], t.Traits[1]}]
	counts := make([]int, len(t.Art.Palette))
	for _, p := range t.Art.Pix {
		counts[p]++
	}
	var skin, all []int
	for k, c := range t.Art.Palette {
		if c != (art.RGB{}) && counts[k] > 0 {
			all = append(all, k)
			if shared[c] {
				skin = append(skin, k)
			}
		}
	}
	if len(skin) == 0 {
		skin = all
	}
	c := largestShade(t.Art.Palette, counts, skin)
	m.swatches[i] = c
	return c
}

func skinColours(tokens []atlas.Token) map[[2]string]map[art.RGB]bool {
	seen := map[[2]string]map[art.RGB]int{}
	refs := map[[2]string]int{}
	for _, t := range tokens {
		if t.Traits[3] != "None" {
			continue
		}
		k := [2]string{t.Traits[0], t.Traits[1]}
		refs[k]++
		if seen[k] == nil {
			seen[k] = map[art.RGB]int{}
		}
		for _, c := range t.Art.Palette {
			seen[k][c]++
		}
	}
	out := map[[2]string]map[art.RGB]bool{}
	skins := map[string][][2]string{}
	for k, cs := range seen {
		out[k] = map[art.RGB]bool{}
		for c, n := range cs {
			if 2*n >= refs[k] {
				out[k][c] = true
			}
		}
		skins[k[0]] = append(skins[k[0]], k)
	}
	for _, ks := range skins {
		if len(ks) < 2 {
			continue
		}
		shared := map[art.RGB]int{}
		for _, k := range ks {
			for c := range out[k] {
				shared[c]++
			}
		}
		for _, k := range ks {
			own := map[art.RGB]bool{}
			for c := range out[k] {
				if 2*shared[c] <= len(ks) {
					own[c] = true
				}
			}
			if len(own) > 0 {
				out[k] = own
			}
		}
	}
	return out
}

func largestShade(pal []art.RGB, counts, idx []int) art.RGB {
	if len(idx) == 0 {
		return art.RGB{}
	}
	sort.SliceStable(idx, func(a, b int) bool { return counts[idx[a]] > counts[idx[b]] })
	var seeds, weights []int
	for _, i := range idx {
		j := slices.IndexFunc(seeds, func(s int) bool { return nearShade(pal[i], pal[s]) })
		if j < 0 {
			seeds, weights = append(seeds, i), append(weights, 0)
			j = len(seeds) - 1
		}
		weights[j] += counts[i]
	}
	best := 0
	for j := range seeds {
		if weights[j] = shadeWeight(pal[seeds[j]], weights[j]); weights[j] > weights[best] {
			best = j
		}
	}
	return pal[seeds[best]]
}

func shadeWeight(c art.RGB, n int) int {
	if int(c[0])*3+int(c[1])*6+int(c[2]) < darkShade*10 {
		return n * 3 / 10
	}
	return n
}

func nearShade(a, b art.RGB) bool {
	d := 0
	for i := range a {
		x := int(a[i]) - int(b[i])
		d += x * x
	}
	return d < shadeDist*shadeDist
}

func bgCell(c art.RGB, s string) string {
	return strings.Replace(fg(c), "38;", "48;", 1) + s + reset
}
