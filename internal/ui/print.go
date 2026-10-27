package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

func (m *Model) applyPrinted(data []any) bool {
	live := false
	for _, v := range data {
		s, ok := v.(*market.Snapshot)
		switch {
		case ok && s == m.printed:
			live = true
		case ok:
			if live = m.apply(v); live {
				m.printed = s
			}
		default:
			m.apply(v)
		}
	}
	return live
}

func roundZec(f float64) string {
	return zec(strconv.FormatFloat(math.Round(f*1000)/1000, 'f', -1, 64))
}

func statZec(s string) string {
	p, ok := search.Price(s)
	if !ok || p < 0 {
		return ""
	}
	if p >= 1000 {
		n, ok := roundInt(p)
		if !ok {
			return ""
		}
		return comma(n) + " ZEC"
	}
	return roundZec(p) + " ZEC"
}

func priceZec(s string) string {
	if !priced(s) {
		return ""
	}
	return zec(s) + " ZEC"
}

func (m *Model) printHead(name, about, url string) []line {
	dim := fg(m.theme.Dim)
	return []line{text(bold(m.theme.Text), name), text(dim, about), text(dim, url), {}}
}

func (m *Model) statLines(groups ...[][3]string) []line {
	keyW, valW := 0, 0
	for _, g := range groups {
		for _, s := range g {
			keyW, valW = max(keyW, width(s[0])), max(valW, width(s[1]))
		}
	}
	var blocks [][]line
	w := 0
	for _, g := range groups {
		var b []line
		for _, s := range g {
			if s[1] == "" {
				continue
			}
			var r row
			r.put(fg(m.theme.Heading), s[0])
			r.padTo(keyW + 3)
			r.put(fg(m.theme.Text), s[1])
			if s[2] != "" {
				r.padTo(keyW + 3 + valW + 2)
				r.put(fg(m.theme.Dim), s[2])
			}
			l := r.line()
			w = max(w, l.w)
			b = append(b, l)
		}
		if len(b) > 0 {
			blocks = append(blocks, b)
		}
	}
	rule := text(fg(mix(m.theme.Border, m.theme.BodyBg, 0.3)), strings.Repeat("-", w))
	var out []line
	for i, b := range blocks {
		if i > 0 {
			out = append(out, rule)
		}
		out = append(out, b...)
	}
	return out
}

func (m *Model) tileField(k, vs, v, right string) line {
	var r row
	r.put(fg(m.theme.Heading), k)
	r.padTo(9)
	r.put(vs, v)
	if right != "" {
		r.space(1)
		r.put(fg(m.theme.Dim), right)
	}
	return r.line()
}

func (m *Model) tile(i int, fields ...line) []line {
	t := &m.tokens[i]
	var out []line
	for _, s := range m.thumb(i) {
		out = append(out, line{s, art.ThumbW})
	}
	out = append(out, line{}, text(bold(m.theme.Text), title(t)))
	out = append(out, fields...)
	rank := "unranked"
	if t.Rank != 0 {
		rank = comma(t.Rank)
	}
	return append(out, m.tileField("RANK:", fg(m.theme.Text), rank, ""))
}

func (m *Model) printPage(cols int, head []line, tiles [][]line, live bool) []string {
	const pad, gap = 2, 3
	textW := art.ThumbW
	for _, t := range tiles {
		for _, l := range t {
			textW = max(textW, l.w)
		}
	}
	colW := max(art.ThumbW, textW-gap+2)
	per := 3
	if cols > 0 {
		per = (cols-2*pad-textW)/(colW+gap) + 1
	}
	per = max(1, min(per, 4, len(tiles)))
	if n := len(tiles); n > 0 {
		rows := (n + per - 1) / per
		per = (n + rows - 1) / rows
	}
	w := 0
	for _, l := range head {
		w = max(w, l.w)
	}
	if len(tiles) > 0 {
		w = max(w, (per-1)*(colW+gap)+textW)
	}
	w += 2 * pad
	blank := onBg(strings.Repeat(" ", w), art.RGB{})
	out := []string{blank}
	emit := func(r *row) {
		r.padTo(w)
		out = append(out, onBg(r.String(), art.RGB{}))
	}
	for _, l := range head {
		var r row
		r.space(pad)
		r.cells(l.s, l.w)
		emit(&r)
	}
	for start := 0; start < len(tiles); start += per {
		out = append(out, blank)
		group := tiles[start:min(start+per, len(tiles))]
		for y := range len(group[0]) {
			var r row
			for c, tile := range group {
				r.padTo(pad + c*(colW+gap))
				if y < len(tile) {
					r.cells(tile[y].s, tile[y].w)
				}
			}
			emit(&r)
		}
	}
	if !live {
		out = append(out, blank)
		var r row
		r.space(pad)
		r.put(fg(m.theme.Dim), "prices as of "+snapshotDate(m.opts.Atlas))
		emit(&r)
	}
	return append(out, blank)
}
