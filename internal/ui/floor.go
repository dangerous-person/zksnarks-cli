package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

// listing is a token for sale and its price.
type listing struct {
	i int
	p float64
}

// cheaper orders listings the way the explorer sorts by price: the rarer token first at
// the same price, so every screen names the same token as the cheapest.
func (m *Model) cheaper(a, b listing) int {
	return search.Cheaper(&m.tokens[a.i], &m.tokens[b.i])
}

// floorMatches reads the words given to the floor command, cleaned like a typed query. It
// returns the tokens the words describe, and those of them that also pass the words about
// the market (listed, price<1, last>2), which only mean something with live prices. No
// words mean the whole collection. A one-shot command cannot ignore what it does not
// understand the way the explorer does while a query is being typed, so an unfinished word
// is an error, and so are words that describe no token at all.
func (m *Model) floorMatches(args []string) (described, matched []int, typed string, err error) {
	var in input
	in.paste(strings.TrimSpace(search.FromArgs(args, m.ix.Cat)))
	typed = in.text
	q := search.Parse(typed, m.ix.Cat)
	if len(q.Errs) > 0 {
		return nil, nil, typed, errors.New(q.Errs[0])
	}
	rest := typed
	var about search.Query
	for _, t := range q.Terms {
		i := strings.Index(rest, t.Text)
		if i < 0 {
			break
		}
		if skipped := strings.TrimSpace(rest[:i]); skipped != "" {
			return nil, nil, typed, fmt.Errorf("nothing after %s", skipped)
		}
		rest = rest[i+len(t.Text):]
		switch {
		case t.Kind == search.SortBy:
			return nil, nil, typed, fmt.Errorf("the floor is sorted by price, leave %s out", t.Text)
		case t.Kind == search.Compare && t.Field != "rank", t.Kind == search.Status && t.Status != "purged" && t.Status != "apparition":
			// about the market
		default:
			about.Terms = append(about.Terms, t)
		}
	}
	if skipped := strings.TrimSpace(rest); skipped != "" {
		return nil, nil, typed, fmt.Errorf("nothing after %s", skipped)
	}
	described = m.ix.Filter(about, m.tokens)
	if len(described) == 0 {
		return nil, nil, typed, fmt.Errorf("no zkSNARK matches %s", typed)
	}
	return described, m.ix.Filter(q, m.tokens), typed, nil
}

// CheckFloor reports what is wrong with the words given to the floor command. Traits ship
// with the build, so this needs no market data.
func (m *Model) CheckFloor(args []string) error {
	_, _, _, err := m.floorMatches(args)
	return err
}

// PrintFloor prints the cheapest listings of the collection, or of the zkSNARKs matching the
// words. At the same price the rarer one comes first.
func (m *Model) PrintFloor(cols int, args []string, data ...any) ([]string, error) {
	live := m.applyPrinted(data)
	described, matched, typed, err := m.floorMatches(args)
	if err != nil {
		return nil, err
	}
	supply := 0
	for _, i := range described {
		if !m.tokens[i].Purged {
			supply++
		}
	}
	var asks []listing
	for _, i := range matched {
		t := &m.tokens[i]
		if p, ok := search.Price(t.Ask); ok && t.Listed && !t.Purged {
			asks = append(asks, listing{i, p})
		}
	}
	slices.SortStableFunc(asks, m.cheaper)
	value, dim := fg(m.theme.Text), fg(m.theme.Dim)
	about := "The cheapest zkSNARKs for sale."
	if typed != "" {
		about = "The cheapest zkSNARKs for sale matching " + typed + "."
		if cols > 0 {
			about = clip(about, cols-2*printPad) // many words would run past the window
		}
	}
	head := m.printHead("FLOOR_", about, "https://zilkroad.com/market")
	if len(asks) == 0 {
		head = append(head, text(dim, "None for sale."))
	} else {
		floor := m.tokens[asks[0].i].Ask
		near := 0
		for _, a := range asks {
			if a.p <= asks[0].p*1.1+1e-9 {
				near++
			}
		}
		head = append(head, m.statLines([][3]string{
			{"FLOOR PRICE:", zec(floor) + " ZEC", m.usdText(floor)},
			{"LISTED:", comma(len(asks)) + " of " + comma(supply), ""},
			{"WITHIN 10%:", comma(near), ""},
		})...)
	}
	now := time.Now()
	var tiles [][]line
	for _, a := range asks[:min(len(asks), 9)] {
		t := &m.tokens[a.i]
		when := "-"
		if at, ok := parseTime(t.ListedAt); ok {
			when = ago(at, now)
		}
		tiles = append(tiles, m.tile(a.i,
			m.tileField("ASKING:", value, zec(t.Ask)+" ZEC", m.usdText(t.Ask)),
			m.tileField("LISTED:", value, when, "")))
	}
	return m.printPage(cols, head, tiles, live), nil
}
