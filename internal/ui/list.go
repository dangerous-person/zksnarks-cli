package ui

import (
	"strings"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

type column struct {
	title string
	w     int
	right bool
	value func(m *Model, i int) string
}

// listColumns are added left to right while they fit; rank comes last before the price.
var listColumns = []column{
	{"LINEAGE", 12, false, func(m *Model, i int) string { return m.tokens[i].Traits[0] }},
	{"SKIN", 11, false, func(m *Model, i int) string { return m.tokens[i].Traits[1] }},
	{"EYES", 14, false, func(m *Model, i int) string { return m.tokens[i].Traits[2] }},
	{"HEAD", 14, false, func(m *Model, i int) string { return m.tokens[i].Traits[3] }},
	{"ACCESSORY", 12, false, func(m *Model, i int) string { return m.tokens[i].Traits[4] }},
	{"RANK", 6, true, func(m *Model, i int) string {
		if r := m.tokens[i].Rank; r > 0 {
			return comma(r)
		}
		return "-"
	}},
}

// listFixed is the width of the table's own columns: cursor 2, swatch 3, number 7, price 7.
const listFixed = 2 + 3 + 7 + 7

// listLayout fits the table into w cells, the trait columns added while they fit, and says
// how wide it is and how wide the preview next to it is: 0 for none, big for the card art.
func (m *Model) listLayout(w, h int) (cols []column, tableW, previewW int, big bool) {
	switch {
	case w >= art.CardW+2+50 && h >= art.CardH+3:
		previewW, big = art.CardW, true
	case w >= art.ThumbW+2+40:
		previewW = art.ThumbW
	}
	room := w - listFixed
	if previewW > 0 {
		room -= previewW + 2
	}
	tableW = listFixed
	for _, c := range listColumns {
		if c.w+2 > room {
			break
		}
		cols = append(cols, c)
		room -= c.w + 2
		tableW += c.w + 2
	}
	return cols, tableW, previewW, big
}

// listLines draws a table of results, the price right after the traits, and a preview of the
// selected token centred in the room left on the right.
func (m *Model) listLines(w, h int) []line {
	if len(m.results) == 0 {
		return m.emptyLines(w, h)
	}
	cols, tableW, previewW, big := m.listLayout(w, h)
	m.keys = nil // the preview's key hints, drawn again below
	out := make([]line, h)
	var hd row
	head := fg(m.theme.ListHead)
	hd.space(5)
	hd.put(head, "#")
	hd.padTo(12)
	for _, c := range cols {
		start := hd.w
		if c.right {
			hd.padTo(start + c.w - width(c.title))
		}
		hd.put(head, c.title)
		hd.padTo(start + c.w + 2)
	}
	hd.padTo(tableW - 3)
	hd.put(head, "ZEC")
	out[0] = hd.line()
	for y := 1; y < h; y++ {
		ri := m.scroll + y - 1
		if ri >= len(m.results) {
			break
		}
		i := m.results[ri]
		t := &m.tokens[i]
		sel := ri == m.sel
		textStyle, valueStyle := fg(m.theme.Dim), fg(m.theme.Dim)
		var r row
		if sel {
			r.put(fg(m.theme.Accent), pointer+" ")
			textStyle, valueStyle = bold(m.theme.Accent), fg(m.theme.Text)
		} else {
			r.space(2)
		}
		r.cells(bgCell(swatch(t.Art), "  "), 2)
		r.space(1)
		r.put(textStyle, "#"+itoa(t.ID))
		r.padTo(12)
		for _, c := range cols {
			v := clip(dashNone(c.value(m, i)), c.w)
			start := r.w
			if c.right {
				r.padTo(start + c.w - width(v))
			}
			style := valueStyle
			if v == "-" { // nothing there stays grey on the selected row too, like the price
				style = fg(m.theme.Dim)
			}
			r.put(style, v)
			r.padTo(start + c.w + 2)
		}
		price, ps := "-", fg(m.theme.Dim)
		switch {
		case t.Purged:
			price, ps = "purged", fg(m.theme.Warn)
		case t.Listed && priced(t.Ask):
			price, ps = zec(t.Ask), textStyle
		}
		r.padTo(tableW - width(price))
		r.put(ps, price)
		if sel { // a bar a cell past the price, so it does not end on the digits
			r.padTo(tableW + 1)
			out[y] = line{onBg(r.String(), m.theme.ListSel), r.w}
			continue
		}
		out[y] = r.line()
	}
	if previewW == 0 {
		return out
	}
	t, ok := m.selected()
	if !ok {
		return out
	}
	var artLines []string
	if big {
		artLines = m.previewArt(m.results[m.sel])
	} else {
		artLines = m.previewThumb(m.results[m.sel])
	}
	x := tableW + max(2, (w-tableW-previewW)/2)
	ox := 2 + x // the frame's border and padding, and the filters panel when it is open
	if m.panel.open {
		ox += panelW + 2
	}
	info := m.previewInfo(t, previewW, h-len(artLines), ox, m.headRows()+len(artLines))
	for y := 0; y < h; y++ {
		var r row
		r.cells(out[y].s, out[y].w)
		r.padTo(x)
		switch {
		case y < len(artLines):
			r.cells(artLines[y], previewW)
		case y-len(artLines) < len(info):
			l := info[y-len(artLines)]
			r.cells(l.s, l.w)
		}
		out[y] = r.line()
	}
	return out
}

// previewInfo is what the list shows under the selected token's art, w wide and at most h
// lines: its name and rank, its traits, its market as the card has it and the card's keys,
// with rules between. (ox, oy) is where it starts on the screen, for the keys' click targets.
// When the traits do not fit one per line they share two lines; the market and the keys are
// left out unless they fit whole.
func (m *Model) previewInfo(t *atlas.Token, w, h, ox, oy int) []line {
	out := []line{{}, leftRight(bold(m.theme.Text), title(t), fg(m.theme.Dim), rankText(t.Rank), w)}
	if len(out)+1+5 > h {
		out = append(out,
			text(fg(m.theme.Text), clip(dashNone(t.Traits[0])+" · "+dashNone(t.Traits[1])+" · "+dashNone(t.Traits[2]), w)),
			text(fg(m.theme.Dim), clip("head "+dashNone(t.Traits[3])+" · "+dashNone(t.Traits[4]), w)))
		return out[:max(0, min(len(out), h))]
	}
	field := func(k, v, vs, right string) line {
		const col = 11 // after "ACCESSORY: "
		var r row
		r.put(fg(m.theme.Heading), k+":")
		r.padTo(col)
		if right != "" && col+width(v)+1+width(right) > w {
			right = ""
		}
		r.put(vs, clip(v, w-col))
		if right != "" {
			r.padTo(w - width(right))
			r.put(fg(m.theme.Dim), right)
		}
		return r.line()
	}
	// a dashed rule, quieter than the frame's lines
	rule := text(fg(mix(m.theme.Border, m.theme.BodyBg, 0.3)), strings.Repeat("-", w))
	white := fg(m.theme.Text)
	out = append(out, rule)
	for i, name := range atlas.TraitNames[:5] {
		out = append(out, field(strings.ToUpper(name), dashNone(t.Traits[i]), white, ""))
	}
	market := []line{rule}
	switch {
	case t.Purged:
		market = append(market, field("STATUS", "purged", fg(m.theme.Warn), ""))
	case t.Listed && priced(t.Ask):
		market = append(market, field("ASKING", zec(t.Ask)+" ZEC", white, m.usdText(t.Ask)))
	default:
		market = append(market, field("STATUS", "not for sale", white, ""))
	}
	if priced(t.Bid) {
		market = append(market, field("BEST BID", zec(t.Bid)+" ZEC", white, m.usdText(t.Bid)))
	}
	if at, ok := parseTime(t.ListedAt); ok && t.Listed {
		market = append(market, field("LISTED", ago(at, time.Now()), white, ""))
	}
	market = append(market, field("LAST SALE", lastText(t.Last), white, ""))
	if len(out)+len(market) > h {
		return out
	}
	out = append(out, market...)
	if len(out)+2 <= h {
		out = append(out, rule, m.cardKeys(len(out)+1, ox, oy, w, m.tokenKeys(t)...))
	}
	return out
}

// pointer marks the selected row of the list, the filters and the completions.
const pointer = "›"

// dashNone writes a trait the token does not have as a dash, like an unlisted price.
func dashNone(v string) string {
	if v == "None" {
		return "-"
	}
	return v
}

func rankText(r int) string {
	if r == 0 {
		return "unranked"
	}
	return "rank " + comma(r)
}
