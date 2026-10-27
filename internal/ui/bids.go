package ui

import (
	"cmp"
	"slices"

	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

type bid struct {
	i int
	p float64
}

func (m *Model) openBids() []bid {
	var out []bid
	for i := range m.tokens {
		if p, ok := search.Price(m.tokens[i].Bid); ok && p > 0 {
			out = append(out, bid{i, p})
		}
	}
	slices.SortStableFunc(out, func(a, b bid) int { return cmp.Compare(b.p, a.p) })
	return out
}

func (m *Model) PrintBids(cols int, data ...any) []string {
	live := m.applyPrinted(data)
	bids := m.openBids()
	value, dim := fg(m.theme.Text), fg(m.theme.Dim)
	head := m.printHead("BIDS_", "zkSNARKs with active bids.", "https://zilkroad.com/bids")
	if len(bids) == 0 {
		head = append(head, text(dim, "No active bids."))
	} else {
		var total float64
		for _, b := range bids {
			total += b.p
		}
		avg := total / float64(len(bids))
		head = append(head, m.statLines([][3]string{
			{"ITEMS WITH BIDS:", comma(len(bids)), ""},
			{"HIGHEST BID:", zec(m.tokens[bids[0].i].Bid) + " ZEC", m.usdOf(bids[0].p)},
			{"AVERAGE OPEN BID:", roundZec(avg) + " ZEC", m.usdOf(avg)},
			{"TOTAL OPEN VALUE:", roundZec(total) + " ZEC", m.usdOf(total)},
		})...)
	}
	var tiles [][]line
	for _, b := range bids[:min(len(bids), 9)] {
		t := &m.tokens[b.i]
		status := m.tileField("STATUS:", dim, "NOT FOR SALE", "")
		switch {
		case t.Purged:
			status = m.tileField("STATUS:", fg(m.theme.Warn), "PURGED", "")
		case t.Listed && priced(t.Ask):
			status = m.tileField("ASKING:", value, zec(t.Ask)+" ZEC", m.usdText(t.Ask))
		}
		tiles = append(tiles, m.tile(b.i, m.tileField("BID:", value, zec(t.Bid)+" ZEC", m.usdText(t.Bid)), status))
	}
	return m.printPage(cols, head, tiles, live)
}
