package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

func TestHelpExamplesFindTokens(t *testing.T) {
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	ix := search.NewIndex(a.Tokens)
	var shown []string
	for _, p := range helpFind {
		shown = append(shown, p[1])
	}
	for _, p := range helpShowSort {
		shown = append(shown, p[1], p[3])
	}
	for _, p := range helpOther {
		shown = append(shown, p[1])
	}
	gap := regexp.MustCompile(` {2,}`)
	for _, s := range shown {
		for _, ex := range gap.Split(s, -1) {
			q := search.Parse(ex, ix.Cat)
			if len(q.Errs) > 0 {
				t.Errorf("%q: %v", ex, q.Errs)
			}
			if s == helpStatuses {
				continue
			}
			if got := len(ix.Run(q, a.Tokens)); got == 0 {
				t.Errorf("%q finds nothing", ex)
			}
		}
	}
	if got := len(ix.Run(search.Parse("#7724 #12", ix.Cat), a.Tokens)); got != 2 {
		t.Errorf("#7724 #12 finds %d", got)
	}
	for _, piece := range strings.Split(queryHint, " · ") {
		if q := search.Parse(piece, ix.Cat); len(q.Errs) > 0 || len(q.Terms) == 0 {
			t.Errorf("hint %q: %v", piece, q.Errs)
		}
	}
}
