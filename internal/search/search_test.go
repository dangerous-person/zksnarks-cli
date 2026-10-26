package search

import (
	"slices"
	"strings"
	"testing"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

func tok(id int, lineage, skin, eyes, head string) atlas.Token {
	return atlas.Token{ID: id, Traits: [6]string{lineage, skin, eyes, head, "None", "Black"}, Rank: id * 10}
}

func fixture() []atlas.Token {
	ts := []atlas.Token{
		tok(1, "Noir", "Dusty Rose", "Volt", "None"),
		tok(2, "Noise", "Moss", "Halogen", "Courier"),
		tok(3, "Skele", "Gold", "Rim Cobalt", "None"),
		tok(4, "Skele", "Violet", "Rim Bone", "Node"),
		tok(5, "Skele", "Violet", "404", "Node Bone"),
		tok(6, "Apparition", "Aura", "Mosaic", "None"),
		tok(7, "Skele", "Gold", "Rim Bone", "None"),
	}
	ts[1].Listed, ts[1].Ask, ts[1].ListedAt = true, "9", "2026-10-09T19:16:49Z"
	ts[2].Listed, ts[2].Ask, ts[2].ListedAt = true, "4.99", "2026-10-16T03:25:33Z"
	ts[3].Listed, ts[3].Ask, ts[3].ListedAt = true, "2.19", "2026-10-18T10:00:00Z"
	ts[5].Rank, ts[5].Apparition = 0, 16
	ts[6].Purged = true
	return ts
}

func ids(ts []atlas.Token, res []int) []int {
	out := make([]int, len(res))
	for i, r := range res {
		out[i] = ts[r].ID
	}
	return out
}

func TestRun(t *testing.T) {
	ts := fixture()
	ix := NewIndex(ts)
	cases := []struct {
		q    string
		want []int
	}{
		{"", []int{1, 2, 3, 4, 5, 6}},            // purged #7 hidden
		{"skele", []int{3, 4, 5}},                // word prefix of a lineage
		{"skele rim", []int{3, 4}},               // words AND
		{"rose", []int{1}},                       // any word inside a value
		{"404", []int{5}},                        // a number is also a word
		{"node", []int{4, 5}},                    // Node and Node Bone
		{"head:node", []int{4}},                  // key:value is exact
		{`eyes:"rim bone"`, []int{4}},            // quotes for spaces, case-insensitive
		{"lineage:noir,noise", []int{1, 2}},      // commas are OR
		{"listed", []int{2, 3, 4}},               // status
		{"price<5", []int{3, 4}},                 // only listed tokens have a price
		{"rank<=30", []int{1, 2, 3}},             // apparitions are unranked
		{"purged", []int{7}},                     // purged only on request
		{"#7 #3", []int{3, 7}},                   // ID terms OR
		{"#7,3", []int{3, 7}},                    // ID list; shows purged
		{"sort:price", []int{4, 3, 2, 1, 5, 6}},  // listed by price, then the rest by number
		{"sort:-price", []int{2, 3, 4, 1, 5, 6}}, // reversed, unlisted still last
		{"sort:-rank", []int{5, 4, 3, 2, 1, 6}},  // unranked last
		{"sort:new", []int{4, 3, 2, 1, 5, 6}},    // newest listing first
		{"skele sort:rank", []int{3, 4, 5}},      // sort with a filter
		{"apparition", []int{6}},                 // status
		{"#7,3 #5", []int{3, 5, 7}},
		{"skele #1 #3", []int{3}},
		{"#1 #3 #4 listed", []int{3, 4}},
		{"#9 #3", []int{3}},
	}
	for _, c := range cases {
		q := Parse(c.q, ix.Cat)
		if len(q.Errs) > 0 {
			t.Errorf("%q: errors %v", c.q, q.Errs)
		}
		if got := ids(ts, ix.Run(q, ts)); !slices.Equal(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
			t.Errorf("%q = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	ix := NewIndex(fixture())
	for _, s := range []string{"colour:red", "eyes:rim", "sort:color", "#abc"} {
		if q := Parse(s, ix.Cat); len(q.Errs) != 1 {
			t.Errorf("%q: errors %v, want one", s, q.Errs)
		}
	}
}

func TestToggle(t *testing.T) {
	ix := NewIndex(fixture())
	q := Parse("skele listed", ix.Cat)
	s := ToggleValue(q, 2, "Rim Bone")
	if s != `skele listed eyes:"Rim Bone"` {
		t.Fatalf("add: %q", s)
	}
	s = ToggleValue(Parse(s, ix.Cat), 2, "Volt")
	if s != `skele listed eyes:"Rim Bone",Volt` {
		t.Fatalf("merge: %q", s)
	}
	s = ToggleValue(Parse(s, ix.Cat), 2, "rim bone")
	if s != "skele listed eyes:Volt" {
		t.Fatalf("remove: %q", s)
	}
	s = ToggleValue(Parse(s, ix.Cat), 2, "Volt")
	if s != "skele listed" {
		t.Fatalf("remove last: %q", s)
	}
	if s = ToggleStatus(Parse(s, ix.Cat), "listed"); s != "skele" {
		t.Fatalf("status: %q", s)
	}
	if s = SetSort(Parse("skele sort:rank", ix.Cat), Sort{"price", true}); s != "skele sort:-price" {
		t.Fatalf("sort: %q", s)
	}
}

func TestFacets(t *testing.T) {
	ts := fixture()
	ix := NewIndex(ts)
	f := ix.Facets(Parse("lineage:skele eyes:\"rim bone\"", ix.Cat), ts)
	// lineage counts ignore the lineage term: Rim Bone among all lineages
	if f[0]["Skele"] != 1 || len(f[0]) != 1 {
		t.Errorf("lineage facet %v", f[0])
	}
	// eyes counts ignore the eyes term: all live Skele eyes
	if f[2]["Rim Bone"] != 1 || f[2]["Rim Cobalt"] != 1 || f[2]["404"] != 1 {
		t.Errorf("eyes facet %v", f[2])
	}
}

func texts(ss []Suggestion) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Text()
	}
	return strings.Join(out, " ")
}

func TestSuggestKeywords(t *testing.T) {
	ts := fixture()
	ix := NewIndex(ts)
	all := ix.Run(Parse("", ix.Cat), ts)
	got := ix.Suggest("", all, ts)
	if s := texts(got); s != "lineage: skin: eyes: head: listed price< rank< sort: # apparition" {
		t.Fatalf("starters: %s", s)
	}
	if got[0].Note != "4 values" || got[4].Count != 3 || got[9].Count != 1 {
		t.Fatalf("starter notes: %+v", got)
	}
	cases := []struct{ in, want string }{
		{"pr", "price< sort:price"},
		{"li", "listed lineage:"},
		{"listed li", "lineage:"},
		{"sort:-", "sort:-price sort:-rank"},
		{"sk", "skin: lineage:Skele"},
	}
	for _, c := range cases {
		res := ix.Run(Parse(c.in, ix.Cat), ts)
		if s := texts(ix.Suggest(c.in, res, ts)); s != c.want {
			t.Errorf("%q: %s, want %s", c.in, s, c.want)
		}
	}
	if got := ix.Suggest("listed li", nil, ts); got[0].Note != "2 values" {
		t.Errorf("lineages among listed: %+v", got)
	}
	for _, c := range []struct {
		in   string
		s    Suggestion
		want string
	}{
		{"", Suggestion{Term: "price<"}, "price<"},
		{"sk", Suggestion{Term: "skin:"}, "skin:"},
		{"skele so", Suggestion{Term: "sort:price"}, "skele sort:price "},
		{"li", Suggestion{Term: "listed"}, "listed "},
	} {
		if got := Complete(c.in, c.s); got != c.want {
			t.Errorf("complete %q with %s: %q", c.in, c.s.Text(), got)
		}
	}
	if q := Parse("price< sort: # rank>=", ix.Cat); len(q.Errs) != 0 || len(q.Terms) != 0 {
		t.Errorf("pending terms: %+v", q)
	}
}

func TestSuggest(t *testing.T) {
	ts := fixture()
	ix := NewIndex(ts)
	q := Parse("skele rim", ix.Cat)
	got := ix.Suggest("skele rim", ix.Run(q, ts), ts)
	if len(got) != 2 || got[0].Value != "Rim Bone" || got[0].Count != 1 || got[1].Value != "Rim Cobalt" {
		t.Fatalf("suggest: %+v", got)
	}
	if c := Complete("skele rim", got[0]); c != `skele eyes:"Rim Bone" ` {
		t.Fatalf("complete: %q", c)
	}
	got = ix.Suggest("4", ix.Run(Parse("4", ix.Cat), ts), ts)
	if len(got) == 0 || got[0].Jump != 4 {
		t.Fatalf("jump: %+v", got)
	}
	if got := ix.Suggest("skele ", nil, ts); got != nil {
		t.Fatalf("finished term: %+v", got)
	}
	got = ix.Suggest("head:no", ix.Run(Parse("", ix.Cat), ts), ts)
	if len(got) != 3 || got[0].Value != "None" {
		t.Fatalf("key suggest: %+v", got)
	}
}
