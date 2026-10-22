package search

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

// Catalogue lists the values of every trait.
type Catalogue struct {
	Values [6][]string          // distinct values per trait, most common first
	Counts [6]map[string]int    // live (not purged) tokens per value
	canon  [6]map[string]string // lower case -> catalogue spelling
	words  map[string][]string  // value -> its lower-case words
}

// Canonical returns the catalogue spelling of a trait value, matched case-insensitively.
func (c *Catalogue) Canonical(k int, v string) (string, bool) {
	s, ok := c.canon[k][strings.ToLower(v)]
	return s, ok
}

// Index evaluates queries over the collection.
type Index struct {
	Cat   *Catalogue
	lower [][6]string // per token: lower-case trait values
	words [][]string  // per token: lower-case words of all its trait values
}

// NewIndex indexes the traits of the tokens. Traits never change after baking, so the index
// is built once; market fields are read from the tokens passed to each call.
func NewIndex(tokens []atlas.Token) *Index {
	cat := &Catalogue{words: map[string][]string{}}
	for k := range cat.Counts {
		cat.Counts[k] = map[string]int{}
		cat.canon[k] = map[string]string{}
	}
	ix := &Index{Cat: cat, lower: make([][6]string, len(tokens)), words: make([][]string, len(tokens))}
	for i, t := range tokens {
		for k, v := range t.Traits {
			if v == "" {
				continue
			}
			low := strings.ToLower(v)
			if _, ok := cat.canon[k][low]; !ok {
				cat.canon[k][low] = v
				cat.Values[k] = append(cat.Values[k], v)
				cat.words[v] = strings.Fields(low)
			}
			if !t.Purged {
				cat.Counts[k][v]++
			}
			ix.lower[i][k] = low
			ix.words[i] = append(ix.words[i], cat.words[v]...)
		}
	}
	for k := range cat.Values {
		sort.SliceStable(cat.Values[k], func(a, b int) bool {
			va, vb := cat.Values[k][a], cat.Values[k][b]
			if cat.Counts[k][va] != cat.Counts[k][vb] {
				return cat.Counts[k][va] > cat.Counts[k][vb]
			}
			return va < vb
		})
	}
	return ix
}

// Price parses a ZEC amount as the API writes it; ok is false for an empty amount.
func Price(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func (ix *Index) match(i int, t *atlas.Token, term *Term) bool {
	switch term.Kind {
	case Word:
		if strings.Contains(term.Word, " ") {
			for _, v := range ix.lower[i] {
				if strings.HasPrefix(v, term.Word) || strings.Contains(v, " "+term.Word) {
					return true
				}
			}
			return false
		}
		for _, w := range ix.words[i] {
			if strings.HasPrefix(w, term.Word) {
				return true
			}
		}
		return false
	case Trait:
		for _, v := range term.Values {
			if ix.lower[i][term.Key] == strings.ToLower(v) {
				return true
			}
		}
		return false
	case ID:
		return slices.Contains(term.IDs, t.ID)
	case Status:
		switch term.Status {
		case "listed":
			return t.Listed
		case "unlisted":
			return !t.Listed
		case "purged":
			return t.Purged
		case "apparition":
			return t.Apparition != 0
		case "auction":
			return t.Auction
		}
	case Compare:
		var v float64
		switch term.Field {
		case "price":
			p, ok := Price(t.Ask)
			if !ok || !t.Listed {
				return false
			}
			v = p
		case "last":
			p, ok := Price(t.Last)
			if !ok {
				return false
			}
			v = p
		case "rank":
			if t.Rank == 0 {
				return false // apparitions are unranked
			}
			v = float64(t.Rank)
		}
		switch term.Op {
		case "<":
			return v < term.Num
		case "<=":
			return v <= term.Num
		case ">":
			return v > term.Num
		case ">=":
			return v >= term.Num
		case "=":
			return v == term.Num
		}
	case SortBy:
		return true
	}
	return false
}

// showsPurged reports whether a query asks for purged tokens.
func (q Query) showsPurged() bool {
	for _, t := range q.Terms {
		if t.Kind == ID || t.Kind == Status && t.Status == "purged" {
			return true
		}
	}
	return false
}

// Filter returns the indices of the tokens matching every term, in token order.
func (ix *Index) Filter(q Query, tokens []atlas.Token) []int {
	purged := q.showsPurged()
	out := make([]int, 0, len(tokens))
	for i := range tokens {
		t := &tokens[i]
		if t.Purged && !purged {
			continue
		}
		ok := true
		for j := range q.Terms {
			if !ix.match(i, t, &q.Terms[j]) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, i)
		}
	}
	return out
}

// Run filters and sorts. Tokens without the sort key (unlisted for price and new, unranked
// for rank) always come last, in number order.
func (ix *Index) Run(q Query, tokens []atlas.Token) []int {
	res := ix.Filter(q, tokens)
	key := func(i int) (float64, bool) {
		t := &tokens[i]
		switch q.Sort.Field {
		case "price":
			p, ok := Price(t.Ask)
			return p, ok && t.Listed
		case "rank":
			return float64(t.Rank), t.Rank != 0
		}
		return float64(t.ID), true
	}
	sort.SliceStable(res, func(a, b int) bool {
		ia, ib := res[a], res[b]
		if q.Sort.Field == "new" {
			ta, tb := &tokens[ia], &tokens[ib]
			okA, okB := ta.Listed && ta.ListedAt != "", tb.Listed && tb.ListedAt != ""
			if okA != okB {
				return okA
			}
			if !okA {
				return ta.ID < tb.ID
			}
			if q.Sort.Desc {
				return ta.ListedAt < tb.ListedAt // oldest listing first
			}
			return ta.ListedAt > tb.ListedAt // ISO 8601 times compare as strings
		}
		ka, okA := key(ia)
		kb, okB := key(ib)
		if okA != okB {
			return okA
		}
		if !okA || ka == kb {
			return tokens[ia].ID < tokens[ib].ID
		}
		if q.Sort.Desc {
			return ka > kb
		}
		return ka < kb
	})
	return res
}

// without returns q minus the terms for which drop is true.
func (q Query) without(drop func(Term) bool) Query {
	out := Query{Sort: q.Sort}
	for _, t := range q.Terms {
		if !drop(t) {
			out.Terms = append(out.Terms, t)
		}
	}
	return out
}

// Facets counts trait values among the results of q. For each trait the query's own term
// for that trait is left out, so the counts show what selecting another value would add.
func (ix *Index) Facets(q Query, tokens []atlas.Token) [6]map[string]int {
	var out [6]map[string]int
	for k := range out {
		out[k] = map[string]int{}
		sub := q.without(func(t Term) bool { return t.Kind == Trait && t.Key == k })
		for _, i := range ix.Filter(sub, tokens) {
			out[k][tokens[i].Traits[k]]++
		}
	}
	return out
}

// StatusCounts counts results per status, each with the query's own term for it left out.
func (ix *Index) StatusCounts(q Query, tokens []atlas.Token) map[string]int {
	out := map[string]int{}
	for _, s := range []string{"listed", "purged", "apparition", "auction"} {
		sub := q.without(func(t Term) bool { return t.Kind == Status && t.Status == s })
		sub.Terms = append(sub.Terms, Term{Kind: Status, Status: s})
		out[s] = len(ix.Filter(sub, tokens))
	}
	return out
}

// Suggestion completes the last, unfinished term of a query.
type Suggestion struct {
	Key   int    // trait index
	Value string // catalogue spelling
	Count int    // results that have this value
	Jump  int    // a token number to jump to instead, 0 for a value
}

// Text is the term that replaces the unfinished one.
func (s Suggestion) Text() string {
	if s.Jump != 0 {
		return "#" + strconv.Itoa(s.Jump)
	}
	return traitText(s.Key, []string{s.Value})
}

// Suggest offers completions for the last term of the input while it is being typed: trait
// values with a word starting with it (counted among results) and, for a number, a jump to
// that token. Background is left out: every token has the same one.
func (ix *Index) Suggest(input string, results []int, tokens []atlas.Token, limit int) []Suggestion {
	if input == "" || strings.HasSuffix(input, " ") {
		return nil
	}
	parts := split(input)
	last := parts[len(parts)-1]
	low := strings.ToLower(last)
	keys := []int{0, 1, 2, 3, 4}
	prefix := low
	if key, val, ok := strings.Cut(low, ":"); ok {
		k, known := traitKeys[key]
		if !known {
			return nil
		}
		keys, prefix = []int{k}, strings.Trim(val, `"`)
		if strings.Contains(prefix, ",") {
			return nil
		}
	} else if strings.HasPrefix(low, "#") || strings.ContainsAny(low, "<>=\"") || statuses[low] {
		return nil
	}
	var out []Suggestion
	if n, err := strconv.Atoi(prefix); err == nil && n >= 1 && n <= len(tokens) && len(keys) > 1 {
		out = append(out, Suggestion{Jump: n})
	}
	type kv struct {
		k int
		v string
	}
	counts := map[kv]int{}
	for _, k := range keys {
		for _, v := range ix.Cat.Values[k] {
			if prefix == "" || wordPrefix(ix.Cat.words[v], prefix) {
				counts[kv{k, v}] = 0
			}
		}
	}
	if len(counts) == 0 {
		return out
	}
	for _, i := range results {
		for _, k := range keys {
			key := kv{k, tokens[i].Traits[k]}
			if _, ok := counts[key]; ok {
				counts[key]++
			}
		}
	}
	var vals []Suggestion
	for key, n := range counts {
		if n > 0 {
			vals = append(vals, Suggestion{Key: key.k, Value: key.v, Count: n})
		}
	}
	sort.Slice(vals, func(a, b int) bool {
		if vals[a].Count != vals[b].Count {
			return vals[a].Count > vals[b].Count
		}
		if vals[a].Key != vals[b].Key {
			return vals[a].Key < vals[b].Key
		}
		return vals[a].Value < vals[b].Value
	})
	out = append(out, vals...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// wordPrefix reports whether the phrase starts at a word of the value.
func wordPrefix(words []string, phrase string) bool {
	pw := strings.Fields(phrase)
	if len(pw) == 0 {
		return true
	}
	for i := range words {
		if len(words)-i < len(pw) {
			return false
		}
		ok := true
		for j, p := range pw {
			if j == len(pw)-1 && !strings.HasSuffix(phrase, " ") {
				ok = ok && strings.HasPrefix(words[i+j], p)
			} else {
				ok = ok && words[i+j] == p
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// Complete replaces the last term of the input with the suggestion.
func Complete(input string, s Suggestion) string {
	parts := split(input)
	if len(parts) == 0 {
		return s.Text() + " "
	}
	parts[len(parts)-1] = s.Text()
	return strings.Join(parts, " ") + " "
}
