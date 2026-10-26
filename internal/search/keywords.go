package search

import (
	"slices"
	"strconv"
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

var notes = map[string]string{
	"listed":      "has an ask price",
	"unlisted":    "no ask price",
	"apparition":  "an apparition",
	"auction":     "on auction now",
	"purged":      "hidden by default",
	"price<":      "asking price in ZEC",
	"last<":       "last sale in ZEC",
	"rank<":       "rarity, 1 is rarest",
	"sort:":       "order the results",
	"#":           "token numbers",
	"sort:number": "by number",
	"sort:price":  "cheapest first",
	"sort:-price": "priciest first",
	"sort:rank":   "rarest first",
	"sort:-rank":  "most common first",
	"sort:new":    "newest listings",
}

var (
	statusOrder = []string{"listed", "unlisted", "apparition", "auction", "purged"}
	compares    = []string{"price", "last", "rank"}
	sortOrder   = []string{"number", "price", "-price", "rank", "-rank", "new"}
)

func term(t string) Suggestion { return Suggestion{Term: t, Note: notes[t]} }

func StatusNote(s string) string { return notes[s] }

func (s Suggestion) open() bool {
	return strings.HasSuffix(s.Term, ":") || strings.HasSuffix(s.Term, "<") || s.Term == "#"
}

func pendingTerm(low string) bool {
	if low == "#" || low == "sort:" {
		return true
	}
	for _, f := range compares {
		for _, op := range []string{"<=", ">=", "<", ">", "="} {
			if low == f+op {
				return true
			}
		}
	}
	return false
}

func keySuggestion(k int, results []int, tokens []atlas.Token) (Suggestion, bool) {
	seen := map[string]bool{}
	for _, i := range results {
		seen[tokens[i].Traits[k]] = true
	}
	note := strconv.Itoa(len(seen)) + " values"
	if len(seen) == 1 {
		note = "1 value"
	}
	return Suggestion{Term: KeyName(k) + ":", Note: note}, len(seen) > 0
}

func (ix *Index) statusSuggestion(s string, base Query, tokens []atlas.Token) (Suggestion, bool) {
	if base.HasStatus(s) {
		return Suggestion{}, false
	}
	q := base
	q.Terms = append(slices.Clip(base.Terms), Term{Kind: Status, Status: s})
	n := len(ix.Filter(q, tokens))
	return Suggestion{Term: s, Note: notes[s], Count: n}, n > 0
}

func (ix *Index) starters(results []int, tokens []atlas.Token) []Suggestion {
	var out []Suggestion
	for k := range 4 {
		if s, ok := keySuggestion(k, results, tokens); ok {
			out = append(out, s)
		}
	}
	if s, ok := ix.statusSuggestion("listed", Query{}, tokens); ok {
		out = append(out, s)
	}
	out = append(out, term("price<"), term("rank<"), term("sort:"), term("#"))
	if s, ok := ix.statusSuggestion("apparition", Query{}, tokens); ok {
		out = append(out, s)
	}
	return out
}

func (ix *Index) keywords(low string, base Query, tokens []atlas.Token) []Suggestion {
	var out []Suggestion
	for _, st := range statusOrder {
		if strings.HasPrefix(st, low) {
			if s, ok := ix.statusSuggestion(st, base, tokens); ok {
				out = append(out, s)
			}
		}
	}
	var results []int
	filtered := false
	for k := range 5 {
		if !strings.HasPrefix(KeyName(k), low) {
			continue
		}
		if !filtered {
			results, filtered = ix.Filter(base, tokens), true
		}
		if s, ok := keySuggestion(k, results, tokens); ok {
			out = append(out, s)
		}
	}
	for _, f := range compares {
		if strings.HasPrefix(f, low) {
			out = append(out, term(f+"<"))
		}
	}
	if strings.HasPrefix("sort", low) {
		out = append(out, term("sort:"))
	}
	for _, f := range sortOrder {
		if strings.HasPrefix(f, low) {
			out = append(out, term("sort:"+f))
		}
	}
	return out
}

func sorts(prefix string) []Suggestion {
	var out []Suggestion
	for _, f := range sortOrder {
		if strings.HasPrefix(f, prefix) {
			out = append(out, term("sort:"+f))
		}
	}
	return out
}
