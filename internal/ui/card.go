package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

func (m *Model) cardKey(k string) tea.Cmd {
	if t, ok := m.selected(); ok && m.histFor == t.ID {
		switch k { // the full history scrolls; ←/→ still step through the tokens
		case "esc", "backspace":
			m.histFor = 0
			return nil
		case "up", "k":
			m.histTop--
			return nil
		case "down", "j":
			m.histTop++
			return nil
		case "pgup", "ctrl+b", "ctrl+u":
			m.histTop -= histPage
			return nil
		case "pgdown", "ctrl+f", "ctrl+d", "space":
			m.histTop += histPage
			return nil
		}
	}
	switch k {
	case "esc", "backspace", "enter":
		m.card = false
		m.ensureVisible()
	case "left", "k", "up":
		if m.sel > 0 {
			m.sel--
		}
	case "right", "l", "j", "down":
		if m.sel < len(m.results)-1 {
			m.sel++
		}
	case "h", "H", "shift+h":
		return m.toggleHistory()
	case "r":
		cmd, _ := m.retryHistory()
		return cmd
	case "z", "o":
		if t, ok := m.selected(); ok {
			return openURL(zilkroadURL(t.ID))
		}
	case "a":
		return m.tokenKey(k)
	}
	return nil
}

// zilkroadURL is the token's page on the marketplace.
func zilkroadURL(id int) string { return "https://zilkroad.com/explorer/" + itoa(id) }

// keyRect is where the card drew a key hint, so a click on it does what the key does.
type keyRect struct {
	rect
	key string
}

// clickKey presses the key whose hint on the card was clicked.
func (m *Model) clickKey(ms tea.Mouse) tea.Cmd {
	for _, k := range m.keys {
		if k.contains(ms.X, ms.Y) {
			return m.cardKey(k.key)
		}
	}
	return nil
}

// cardKeys draws "[KEY] LABEL" pairs on line y of the info column, whose top left corner
// is at (ox, oy) on the screen, and records each pair as a click target. When the pairs do
// not fit in w cells, the short labels are used.
func (m *Model) cardKeys(y, ox, oy, w int, pairs ...[3]string) line {
	draw := func(label int) (line, []keyRect) {
		var r row
		var keys []keyRect
		for i, p := range pairs {
			if i > 0 {
				r.space(2)
			}
			x0 := r.w
			m.keyHint(&r, p[0])
			r.space(1)
			r.put(fg(m.theme.HintLabel), strings.ToUpper(p[label]))
			keys = append(keys, keyRect{rect{ox + x0, oy + y, ox + r.w, oy + y + 1}, p[0]})
		}
		return r.line(), keys
	}
	l, keys := draw(1)
	if l.w > w {
		l, keys = draw(2)
	}
	m.keys = append(m.keys, keys...)
	return l
}

// cardInfo is the token's details: a heading (unless the frame already carries it), the
// traits as the site shows them, the market and the keys to the token's page and history;
// or, while it is open, the token's full history. (ox, oy) is where the column starts on
// the screen, for the click targets.
func (m *Model) cardInfo(t *atlas.Token, w, h int, titled bool, ox, oy int) []line {
	var out []line
	add := func(l line) { out = append(out, l) }
	if titled {
		add(text(bold(m.theme.Text), title(t)))
	}
	meta := &m.opts.Atlas.Meta
	switch {
	case t.Purged:
	case t.Apparition != 0:
		msg := "apparition " + itoa(t.Apparition)
		if ap, ok := meta.ApparitionBySlot(t.Apparition); ok {
			msg = ap.Name
			if at, ok := parseTime(ap.UnlockedAt); ok {
				msg += " · unveiled " + at.Local().Format("Jan 2")
			}
		}
		add(text(fg(m.theme.Accent), clip(msg, w)))
	case t.Auction:
		add(text(fg(m.theme.Accent), "on auction"))
	}
	if len(out) > 0 {
		add(line{})
	}
	if m.histFor == t.ID {
		out = append(out, m.historyView(t, w, h-len(out), ox, oy+len(out))...)
		return out[:max(0, min(len(out), h))]
	}
	if heading, label := "TRAITS_", "Floor in ZEC"; width(heading)+2+width(label) <= w {
		add(leftRight(bold(m.theme.Text), heading, fg(m.theme.TraitFloor), label, w))
	} else {
		add(text(bold(m.theme.Text), heading))
	}
	for _, l := range m.traitGrid(t, w) {
		add(l)
	}
	add(line{})
	if t.Purged {
		add(text(bold(m.theme.Text), "PURGED_"))
		if ap, ok := meta.ApparitionBySlot(t.PurgedFor); ok {
			add(kv(m, "apparition", twoDigits(ap.Slot)+" #"+itoa(ap.Token)+"_", "", w))
			if at, ok := parseTime(ap.UnlockedAt); ok {
				add(kv(m, "date", at.Local().Format("Jan 2, 2006"), "", w))
			}
		} else {
			add(kv(m, "status", "purged", "", w))
		}
	} else {
		add(text(bold(m.theme.Text), "MARKET_"))
		if t.Listed && priced(t.Ask) {
			add(kv(m, "asking", zec(t.Ask)+" ZEC", m.usdText(t.Ask), w))
		} else {
			add(kv(m, "status", "not for sale", "", w))
		}
		if priced(t.Bid) {
			add(kv(m, "best bid", zec(t.Bid)+" ZEC", m.usdText(t.Bid), w))
		}
		if at, ok := parseTime(t.ListedAt); ok && t.Listed {
			add(kv(m, "listed", ago(at, time.Now()), "", w))
		}
		add(kv(m, "last sale", lastText(t.Last), "", w))
	}
	if m.artChanged[t.ID] {
		add(text(fg(m.theme.Warn), clip("art changed since this build", w)))
	}
	add(line{})
	if len(out) < h {
		add(m.cardKeys(len(out), ox, oy, w, m.tokenKeys(t)...))
	}
	return out[:max(0, min(len(out), h))]
}

func (m *Model) tokenKeys(t *atlas.Token) [][3]string {
	if ap, ok := m.burntFor(t); ok {
		show := "show #" + itoa(ap.Token) + "_"
		return [][3]string{{"a", show, show}, {"z", "zilkroad", "zilkroad"}, {"h", "history", "history"}}
	}
	return [][3]string{{"z", "open zilkroad", "zilkroad"}, {"h", "show history", "history"}}
}

func (m *Model) burntFor(t *atlas.Token) (atlas.Apparition, bool) {
	if !t.Purged || t.PurgedFor == 0 {
		return atlas.Apparition{}, false
	}
	return m.opts.Atlas.Meta.ApparitionBySlot(t.PurgedFor)
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// title is the token's name the way the site writes it.
func title(t *atlas.Token) string { return "zkSNARK #" + itoa(t.ID) + "_" }

// traitGrid draws the five traits like the site's trait cards: one frame of two columns
// and three rows of sections. Each trait section has the trait's name, its value, its share
// of the live supply and the cheapest listing with that value; the sixth holds the rarity
// rank and score. Narrow columns get one section per row.
func (m *Model) traitGrid(t *atlas.Token, w int) []line {
	cols := 2
	if w < 36 {
		cols = 1
	}
	// section widths between the lines; the last column takes the remainder so the frame
	// spans exactly w
	widths := make([]int, cols)
	for c := range widths {
		widths[c] = (w - cols - 1) / cols
	}
	widths[cols-1] = w - cols - 1 - (cols-1)*widths[0]
	const cells = 6
	b := fg(m.theme.TraitBorder)
	edge := func(l, mid, r string) line {
		var x row
		x.put(b, l)
		for c, cw := range widths {
			if c > 0 {
				x.put(b, mid)
			}
			x.put(b, strings.Repeat("─", cw))
		}
		x.put(b, r)
		return x.line()
	}
	supply := m.opts.Atlas.Meta.Supply
	out := []line{edge("┌", "┬", "┐")}
	for start := 0; start < cells; start += cols {
		for k := 0; k < 4; k++ { // name, value, a gap, share and floor
			var r row
			r.put(b, "│")
			for c, cw := range widths {
				i := start + c
				x0 := r.w
				inner := cw - 2
				if i == 5 {
					r.space(1)
					m.rankSection(&r, t, k, x0, inner)
				}
				if i < 5 {
					r.space(1)
					v := t.Traits[i]
					switch k {
					case 0:
						r.put(fg(m.theme.Heading), clip(strings.ToUpper(atlas.TraitNames[i]), inner-2))
						r.padTo(x0 + 1 + inner - 1)
						r.put(fg(m.theme.Dim), "↗")
					case 1:
						r.put(bold(m.theme.Text), clip(v, inner))
					case 3:
						share := percent2(m.ix.Cat.Counts[i][v], supply)
						f := "-"
						if p, ok := m.floors[i][v]; ok {
							f = strconv.FormatFloat(p, 'f', -1, 64) + " ZEC"
						}
						r.put(fg(m.theme.Dim), share)
						if width(share)+1+width(f) <= inner {
							r.padTo(x0 + 1 + inner - width(f))
							r.put(fg(m.theme.TraitFloor), f)
						}
					}
				}
				r.padTo(x0 + cw)
				r.put(b, "│")
			}
			out = append(out, r.line())
		}
		if start+cols < cells {
			out = append(out, edge("├", "┼", "┤"))
		}
	}
	return append(out, edge("└", "┴", "┘"))
}

// usdText is a ZEC amount in dollars at the live rate, or nothing without one.
func (m *Model) usdText(amount string) string {
	if p, ok := search.Price(amount); ok && m.rate != nil {
		if s := usd(p * m.rate.ZecUsd); s != "" {
			return "≈ " + s
		}
	}
	return ""
}

func kv(m *Model, k, v, right string, w int) line {
	var r row
	r.put(fg(m.theme.Heading), k)
	r.padTo(12)
	r.put(fg(m.theme.Text), clip(v, w-12-width(right)-1))
	if right != "" {
		r.padTo(w - width(right))
		r.put(fg(m.theme.Dim), right)
	}
	return r.line()
}

func lastText(s string) string {
	if !priced(s) {
		return "-"
	}
	return zec(s) + " ZEC"
}

func percent2(n, total int) string {
	if total == 0 {
		return "-"
	}
	return strconv.FormatFloat(float64(n)*100/float64(total), 'f', 2, 64) + "%"
}

func trimFloat(f float64) string {
	n, ok := roundInt(f * 100)
	if !ok {
		return ""
	}
	return zecFloat(float64(n) / 100)
}

// rankSection fills the grid's sixth section: the rank out of all minted tokens and, where
// it fits, the score the rank comes from (the sum of -ln(share) over the traits, as the API
// computes it).
func (m *Model) rankSection(r *row, t *atlas.Token, k, x0, inner int) {
	switch k {
	case 0:
		r.put(fg(m.theme.Heading), "RANK")
	case 1:
		if t.Rank == 0 {
			r.put(bold(m.theme.Text), "unranked")
		} else {
			r.put(bold(m.theme.Text), comma(t.Rank))
		}
	case 3:
		of := ""
		if t.Rank != 0 {
			of = "of " + comma(m.opts.Atlas.Meta.Minted)
			r.put(fg(m.theme.Dim), of)
		}
		if s := trimFloat(t.Score); t.Score > 0 && s != "" {
			if sc := "score " + s; width(of)+1+width(sc) <= inner {
				r.padTo(x0 + 1 + inner - width(sc))
				r.put(fg(m.theme.Dim), sc)
			}
		}
	}
}
