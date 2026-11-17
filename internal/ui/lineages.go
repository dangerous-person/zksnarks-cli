package ui

import (
	"cmp"
	"math"
	"slices"

	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

// PrintLineages prints every lineage with its live supply, its listings, its floor and the
// token that sets it, the cheapest lineage first. At the same floor the smaller lineage
// comes first; lineages with nothing for sale come last.
func (m *Model) PrintLineages(cols int, data ...any) []string {
	live := m.applyPrinted(data)
	type lineage struct {
		name         string
		live, listed int
		floor        listing // the cheapest listing, when listed is not 0
	}
	byName := map[string]*lineage{}
	var all []*lineage
	for i := range m.tokens {
		t := &m.tokens[i]
		if t.Purged || t.Traits[0] == "" {
			continue
		}
		l := byName[t.Traits[0]]
		if l == nil {
			l = &lineage{name: t.Traits[0]}
			byName[l.name] = l
			all = append(all, l)
		}
		l.live++
		if p, ok := search.Price(t.Ask); ok && t.Listed {
			if ask := (listing{i, p}); l.listed == 0 || m.cheaper(ask, l.floor) < 0 {
				l.floor = ask
			}
			l.listed++
		}
	}
	price := func(l *lineage) float64 {
		if l.listed == 0 {
			return math.Inf(1)
		}
		return l.floor.p
	}
	slices.SortStableFunc(all, func(a, b *lineage) int {
		return cmp.Or(cmp.Compare(price(a), price(b)), cmp.Compare(a.live, b.live), cmp.Compare(a.name, b.name))
	})

	const gap = 2
	// name, live, listed, floor, its dollar price, the token that sets the floor
	titles := [6]string{"LINEAGE", "LIVE", "LISTED", "FLOOR", "", "CHEAPEST"}
	var w [6]int
	rows := make([][6]string, len(all))
	for i, l := range all {
		rows[i] = [6]string{l.name, comma(l.live), comma(l.listed), "-"}
		if l.listed > 0 {
			t := &m.tokens[l.floor.i]
			rows[i][3], rows[i][4], rows[i][5] = zec(t.Ask)+" ZEC", m.usdText(t.Ask), "#"+itoa(t.ID)
		}
	}
	var liveEnd, listedEnd, floorAt, tokenAt int
	measure := func() int {
		w = [6]int{}
		for _, c := range append([][6]string{titles}, rows...) {
			for k, s := range c {
				w[k] = max(w[k], width(s))
			}
		}
		liveEnd = w[0] + gap + w[1]
		listedEnd = liveEnd + gap + w[2]
		floorAt = listedEnd + gap
		tokenAt = floorAt + w[3] + gap
		if w[4] > 0 {
			tokenAt += w[4] + gap
		}
		return tokenAt + w[5] + 2*printPad
	}
	if cols > 0 && measure() > cols { // a narrow window loses the dollar prices
		for i := range rows {
			rows[i][4] = ""
		}
	}
	measure()
	draw := func(style string, c [6]string) line {
		var r row
		r.put(style, c[0])
		r.padTo(liveEnd - width(c[1]))
		r.put(style, c[1])
		r.padTo(listedEnd - width(c[2]))
		r.put(style, c[2])
		r.padTo(floorAt)
		r.put(style, c[3])
		if c[4] != "" {
			r.padTo(floorAt + w[3] + gap)
			r.put(fg(m.theme.Dim), c[4])
		}
		if c[5] != "" {
			r.padTo(tokenAt)
			r.put(style, c[5])
		}
		return r.line()
	}
	head := m.printHead("LINEAGES_", "The floor of every lineage.", "https://zilkroad.com/market")
	head = append(head, draw(fg(m.theme.Heading), titles))
	for _, c := range rows {
		head = append(head, draw(fg(m.theme.Text), c))
	}
	return m.printPage(cols, head, nil, live)
}
