package ui

import (
	"math"
	"testing"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
)

func TestSnapshotRank(t *testing.T) {
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{Atlas: a, Client: market.New(true)})
	i := m.byID[907]
	baked := m.tokens[i].Rank
	cases := []struct {
		rank, want int
	}{
		{-1, baked},
		{math.MinInt64, baked},
		{a.Meta.Minted + 1, baked},
		{math.MaxInt64, baked},
		{42, 42},
		{a.Meta.Minted, a.Meta.Minted},
		{0, 0},
	}
	for _, c := range cases {
		m.tokens[i].Rank = baked
		r := c.rank
		m.applySnapshot(&market.Snapshot{Tokens: []market.Token{{ID: 907, Rank: &r}}})
		if got := m.tokens[i].Rank; got != c.want {
			t.Errorf("rank %d: got %d, want %d", c.rank, got, c.want)
		}
	}
}
