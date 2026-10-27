package ui

import (
	"slices"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/market"
)

func (m *Model) PrintSales(cols int, sales []market.Sale, data ...any) []string {
	m.applyPrinted(data)
	value, dim := fg(m.theme.Text), fg(m.theme.Dim)
	type sold struct {
		i  int
		s  market.Sale
		at time.Time
	}
	var recent []sold
	for _, s := range sales {
		i, ok := m.byID[s.ID]
		if !ok || !priced(string(s.Price)) {
			continue
		}
		at, _ := parseTime(s.Time)
		recent = append(recent, sold{i, s, at})
	}
	slices.SortStableFunc(recent, func(a, b sold) int { return b.at.Compare(a.at) })
	recent = recent[:min(len(recent), 9)]
	about := "Completed zkSNARK trades on Zilkroad."
	if len(recent) > 1 {
		about = "Last " + itoa(len(recent)) + " completed zkSNARK trades on Zilkroad."
	}
	head := m.printHead("SALES_", about, "https://zilkroad.com/sales")
	if s := m.stats; s != nil {
		head = append(head, m.statLines([][3]string{
			{"24H SALES:", comma(s.Sales24h), ""},
			{"24H VOLUME:", statZec(string(s.Volume24h)), m.usdText(string(s.Volume24h))},
			{"FLOOR PRICE:", priceZec(string(s.Floor)), m.usdText(string(s.Floor))},
		}, [][3]string{
			{"TOTAL SALES:", comma(s.SalesTotal), ""},
			{"TOTAL VOLUME:", statZec(string(s.VolumeTotal)), m.usdText(string(s.VolumeTotal))},
			{"MARKET CAP:", statZec(string(s.MarketCap)), m.usdText(string(s.MarketCap))},
		})...)
	}
	if len(recent) == 0 {
		head = append(head, text(dim, "No sales yet."))
	}
	now := time.Now()
	var tiles [][]line
	for _, r := range recent {
		when := "-"
		if !r.at.IsZero() {
			when = ago(r.at, now)
		}
		tiles = append(tiles, m.tile(r.i,
			m.tileField("PRICE:", value, zec(string(r.s.Price))+" ZEC", m.usdText(string(r.s.Price))),
			m.tileField("SOLD:", value, when, "")))
	}
	return m.printPage(cols, head, tiles, true)
}
