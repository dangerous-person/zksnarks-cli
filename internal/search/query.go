// Package search parses the query language and evaluates it over the collection.
//
//	skele rim            words: each must start a word of some trait value (AND)
//	#7724 #12            token numbers (OR)
//	eyes:"rim cobalt"    one trait, exact value; commas mean OR: lineage:noise,dune
//	listed purged        status: listed, unlisted, purged, apparition, auction
//	price<1.5 rank<=100  numbers: price, last, rank with < <= > >= =
//	sort:price           sort: number, price, rank, new; a leading - reverses
//
// Purged tokens are hidden unless the query asks for them with "purged" or a #number.
package search

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

// Kind is the kind of a query term.
type Kind int

const (
	Word Kind = iota
	Trait
	ID
	Status
	Compare
	SortBy
)

// Term is one space-separated part of a query.
type Term struct {
	Kind   Kind
	Text   string   // as written, used to rebuild the query
	Word   string   // Word: lower case
	Key    int      // Trait: index into atlas.TraitNames
	Values []string // Trait: values as written in the catalogue
	IDs    []int    // ID
	Status string   // Status
	Field  string   // Compare: price, last or rank
	Op     string   // Compare: < <= > >= =
	Num    float64  // Compare
}

// Sort orders results.
type Sort struct {
	Field string // number, price, rank or new
	Desc  bool
}

// Query is a parsed query.
type Query struct {
	Terms []Term
	Sort  Sort
	Errs  []string // problems to show the user; the offending terms are ignored
}

var traitKeys = map[string]int{
	"lineage": 0, "l": 0,
	"skin": 1, "s": 1,
	"eyes": 2, "eye": 2, "e": 2,
	"head": 3, "h": 3,
	"accessory": 4, "acc": 4, "a": 4,
	"background": 5, "bg": 5,
}

var statuses = map[string]bool{"listed": true, "unlisted": true, "purged": true, "apparition": true, "auction": true}

var sortFields = map[string]string{
	"number": "number", "id": "number", "#": "number",
	"price": "price", "rank": "rank", "rarity": "rank", "new": "new", "recent": "new",
}

// KeyName returns the query key for a trait index.
func KeyName(k int) string { return strings.ToLower(atlas.TraitNames[k]) }

// split cuts a query into terms at spaces outside double quotes.
func split(s string) []string {
	var out []string
	var b strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			b.WriteRune(r)
		case r == ' ' && !quoted:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// splitValues cuts a,"b c",d at commas outside quotes and drops the quotes.
func splitValues(s string) []string {
	var out []string
	var b strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ',' && !quoted:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	return append(out, b.String())
}

// Parse parses a query. The catalogue validates trait values and maps them to their
// canonical spelling.
func Parse(s string, cat *Catalogue) Query {
	q := Query{Sort: Sort{Field: "number"}}
	for _, raw := range split(s) {
		low := strings.ToLower(raw)
		switch {
		case strings.HasPrefix(low, "sort:"):
			f := strings.TrimPrefix(low, "sort:")
			desc := strings.HasPrefix(f, "-")
			f = strings.TrimPrefix(f, "-")
			field, ok := sortFields[f]
			if !ok {
				q.Errs = append(q.Errs, fmt.Sprintf("unknown sort %q (number, price, rank, new)", f))
				continue
			}
			q.Sort = Sort{field, desc}
			q.Terms = append(q.Terms, Term{Kind: SortBy, Text: raw})
		case strings.HasPrefix(raw, "#"):
			var ids []int
			ok := true
			for _, p := range strings.Split(raw[1:], ",") {
				n, err := strconv.Atoi(p)
				if err != nil || n < 1 {
					ok = false
					break
				}
				ids = append(ids, n)
			}
			if !ok || len(ids) == 0 {
				q.Errs = append(q.Errs, fmt.Sprintf("%q is not a token number", raw))
				continue
			}
			q.Terms = append(q.Terms, Term{Kind: ID, Text: raw, IDs: ids})
		case statuses[low]:
			q.Terms = append(q.Terms, Term{Kind: Status, Text: raw, Status: low})
		case strings.Contains(low, ":"):
			key, val, _ := strings.Cut(raw, ":")
			k, ok := traitKeys[strings.ToLower(key)]
			if !ok {
				q.Errs = append(q.Errs, fmt.Sprintf("unknown key %q", key))
				continue
			}
			var values []string
			for _, v := range splitValues(val) {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				canon, ok := cat.Canonical(k, v)
				if !ok {
					q.Errs = append(q.Errs, fmt.Sprintf("%s has no value %q", KeyName(k), v))
					continue
				}
				values = append(values, canon)
			}
			if len(values) == 0 {
				continue
			}
			q.Terms = append(q.Terms, Term{Kind: Trait, Text: raw, Key: k, Values: values})
		default:
			if t, ok := parseCompare(raw); ok {
				q.Terms = append(q.Terms, t)
				continue
			}
			w := strings.Trim(low, `"`)
			if w == "" {
				continue
			}
			q.Terms = append(q.Terms, Term{Kind: Word, Text: raw, Word: w})
		}
	}
	return q
}

func parseCompare(raw string) (Term, bool) {
	low := strings.ToLower(raw)
	for _, field := range []string{"price", "last", "rank"} {
		if !strings.HasPrefix(low, field) {
			continue
		}
		rest := low[len(field):]
		for _, op := range []string{"<=", ">=", "<", ">", "="} {
			if strings.HasPrefix(rest, op) {
				n, err := strconv.ParseFloat(rest[len(op):], 64)
				if err != nil {
					return Term{}, false
				}
				return Term{Kind: Compare, Text: raw, Field: field, Op: op, Num: n}, true
			}
		}
	}
	return Term{}, false
}

// String rebuilds the query text from its terms.
func (q Query) String() string {
	parts := make([]string, len(q.Terms))
	for i, t := range q.Terms {
		parts[i] = t.Text
	}
	return strings.Join(parts, " ")
}

// traitText writes a trait term, quoting values that contain spaces or commas.
func traitText(k int, values []string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		if strings.ContainsAny(v, " ,") {
			v = `"` + v + `"`
		}
		parts[i] = v
	}
	return KeyName(k) + ":" + strings.Join(parts, ",")
}

// HasValue reports whether a trait term selects exactly this value.
func (q Query) HasValue(k int, value string) bool {
	for _, t := range q.Terms {
		if t.Kind == Trait && t.Key == k {
			for _, v := range t.Values {
				if strings.EqualFold(v, value) {
					return true
				}
			}
		}
	}
	return false
}

// HasStatus reports whether the query has a status term.
func (q Query) HasStatus(s string) bool {
	for _, t := range q.Terms {
		if t.Kind == Status && t.Status == s {
			return true
		}
	}
	return false
}

// ToggleValue adds the value to the query's term for trait k, or removes it if present, and
// returns the new query text. Other terms keep their text.
func ToggleValue(q Query, k int, value string) string {
	for i, t := range q.Terms {
		if t.Kind != Trait || t.Key != k {
			continue
		}
		var values []string
		found := false
		for _, v := range t.Values {
			if strings.EqualFold(v, value) {
				found = true
				continue
			}
			values = append(values, v)
		}
		if !found {
			values = append(values, value)
		}
		terms := append([]Term{}, q.Terms...)
		if len(values) == 0 {
			terms = append(terms[:i], terms[i+1:]...)
		} else {
			terms[i] = Term{Kind: Trait, Text: traitText(k, values), Key: k, Values: values}
		}
		return Query{Terms: terms}.String()
	}
	terms := append(append([]Term{}, q.Terms...), Term{Kind: Trait, Text: traitText(k, []string{value}), Key: k, Values: []string{value}})
	return Query{Terms: terms}.String()
}

// ToggleStatus adds or removes a status term and returns the new query text.
func ToggleStatus(q Query, s string) string {
	var terms []Term
	found := false
	for _, t := range q.Terms {
		if t.Kind == Status && t.Status == s {
			found = true
			continue
		}
		terms = append(terms, t)
	}
	if !found {
		terms = append(terms, Term{Kind: Status, Text: s, Status: s})
	}
	return Query{Terms: terms}.String()
}

// SetSort replaces the query's sort term and returns the new query text.
func SetSort(q Query, s Sort) string {
	var terms []Term
	for _, t := range q.Terms {
		if t.Kind != SortBy {
			terms = append(terms, t)
		}
	}
	if s.Field != "number" || s.Desc {
		text := "sort:"
		if s.Desc {
			text += "-"
		}
		terms = append(terms, Term{Kind: SortBy, Text: text + s.Field})
	}
	return Query{Terms: terms}.String()
}
