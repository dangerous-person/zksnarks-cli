package ui

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

func openZilkroad(m *Model, grid bool) {
	m.section = secZilkroad
	if grid {
		m.mode = modeGallery
	}
	m.ensureVisible()
}

func typeText(m *Model, s string) {
	for _, c := range s {
		m.Update(tea.KeyPressMsg{Code: c, Text: string(c)})
	}
}

func TestStartsOnTheFrontPage(t *testing.T) {
	var st State
	old := `{"query": "skele rim", "grid": true, "theme": "zec", "section": "zilkroad"}`
	if err := json.Unmarshal([]byte(old), &st); err != nil {
		t.Fatal(err)
	}
	m := colourModel(t, Options{State: st})
	if m.section != secRoot || m.input.text != "" || m.mode != modeList || m.theme.Name != "zec" {
		t.Fatalf("section %s, query %q, mode %v, theme %q", sectionNames[m.section], m.input.text, m.mode, m.theme.Name)
	}
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	typeText(m, "skele")
	data, err := json.Marshal(m.SavedState())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"theme":"zec"}` {
		t.Errorf("saved %s", data)
	}
}

func TestQueryCap(t *testing.T) {
	long := strings.Repeat("a", maxQuery+100)
	var in input
	in.set(long)
	if len(in.r) != maxQuery || in.text != long[:maxQuery] || in.pos != maxQuery {
		t.Fatalf("set: %d runes, pos %d", len(in.r), in.pos)
	}
	if in.key(tea.KeyPressMsg{Code: 'b', Text: "b"}) || in.paste("bc") {
		t.Error("text grew past the cap")
	}
	in.set(strings.Repeat("é", maxQuery-2))
	in.pos = 0
	in.paste("xyz")
	if in.text != "xy"+strings.Repeat("é", maxQuery-2) || in.pos != 2 {
		t.Errorf("paste at the cap: %q, pos %d", in.text, in.pos)
	}
	m := colourModel(t, Options{Query: long})
	if len(m.input.r) != maxQuery {
		t.Errorf("-q kept %d runes", len(m.input.r))
	}
	openZilkroad(m, false)
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(tea.PasteMsg{Content: strings.Repeat("z", 300)})
	if len(m.input.r) != maxQuery {
		t.Errorf("paste kept %d runes", len(m.input.r))
	}
}

func TestPaste(t *testing.T) {
	var in input
	cases := []struct{ paste, want string }{
		{"#7724\r\n#12", "#7724 #12"},
		{"skele\t\t rim", "skele rim"},
		{"a\x1b[31mb\x07c", "a[31mbc"},
		{"  lead\n\ntrail \r\n", " lead trail "},
		{"rim cobalt", "rim cobalt"},
		{"\x00\x1b", ""},
	}
	for _, c := range cases {
		in.set("")
		in.paste(c.paste)
		if in.text != c.want {
			t.Errorf("paste %q = %q, want %q", c.paste, in.text, c.want)
		}
	}
	in.set("skele cobalt")
	in.pos = 6
	if !in.paste("rim ") || in.text != "skele rim cobalt" || in.pos != 10 {
		t.Errorf("paste at the cursor: %q, pos %d", in.text, in.pos)
	}

	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.Update(tea.PasteMsg{Content: "skele"})
	if m.input.text != "" {
		t.Fatalf("paste without focus: %q", m.input.text)
	}
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.PasteMsg{Content: "#7724\n#12"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.input.text != "#7724 #12" || len(m.results) != 2 {
		t.Errorf("query %q, %d results", m.input.text, len(m.results))
	}
}

func TestSuggestionStarters(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if len(m.sugg) == 0 || m.sugg[0].Term != "lineage:" || !strings.Contains(plainView(m)[m.headRows()+1], "› lineage:     22 values") {
		t.Fatalf("starters: %+v", m.sugg)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.input.text != "lineage:" || len(m.sugg) != 22 || m.sugg[0].Key != 0 || len(m.q.Errs) != 0 {
		t.Fatalf("after tab: %q, %d suggestions, %v", m.input.text, len(m.sugg), m.q.Errs)
	}
	m.input.set("")
	typeText(m, "so")
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.input.text != "sort:" || len(m.sugg) != 6 || len(m.q.Errs) != 0 {
		t.Fatalf("sort: %q, %d suggestions, %v", m.input.text, len(m.sugg), m.q.Errs)
	}
	m.input.set("")
	typeText(m, "skin:")
	first := m.sugg[0]
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if want := search.Complete("skin:", first); m.input.text != want || m.focus != focusQuery || len(m.sugg) != 0 {
		t.Fatalf("enter: %q, want %q, focus %v", m.input.text, want, m.focus)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.focus != focusMain {
		t.Fatalf("second enter: focus %v", m.focus)
	}
}

func TestSuggestionsScroll(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	typeText(m, "skin:")
	n := len(m.sugg)
	if n <= suggRows || m.suggestionLines() != suggRows+2 {
		t.Fatalf("%d suggestions, %d lines", n, m.suggestionLines())
	}
	top := m.headRows()
	if m.suggSel != 0 || !strings.Contains(plainView(m)[top+1], "› "+m.sugg[0].Value) {
		t.Fatalf("first suggestion not picked: sel %d", m.suggSel)
	}
	for range 8 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	box := plainView(m)[top : top+suggRows+2]
	if m.suggSel != 8 || m.suggTop != 3 || !strings.Contains(box[suggRows], "› "+m.sugg[8].Value) || !strings.Contains(box[suggRows+1], "─ 9 of "+itoa(n)+" ─╯") {
		t.Fatalf("sel %d, top %d, box:\n%s", m.suggSel, m.suggTop, strings.Join(box, "\n"))
	}
	picked := m.sugg[m.suggSel].Text()
	m.refresh(true)
	if m.suggSel != 8 || m.suggTop != 3 || m.sugg[m.suggSel].Text() != picked {
		t.Fatalf("refresh moved the pick: sel %d, top %d", m.suggSel, m.suggTop)
	}
	m.Update(tea.MouseWheelMsg{X: 20, Y: top + 2, Button: tea.MouseWheelDown})
	if m.suggTop != 4 || m.suggSel != 8 {
		t.Fatalf("one wheel step: sel %d, top %d", m.suggSel, m.suggTop)
	}
	for range n {
		m.Update(tea.MouseWheelMsg{X: 20, Y: top + 2, Button: tea.MouseWheelDown})
	}
	if m.suggTop != n-suggRows || m.suggSel != n-suggRows {
		t.Fatalf("wheel to the end: sel %d, top %d", m.suggSel, m.suggTop)
	}
	scroll := m.scroll
	m.Update(tea.MouseWheelMsg{X: 20, Y: top + suggRows + 5, Button: tea.MouseWheelDown})
	if m.scroll == scroll || m.suggTop != n-suggRows {
		t.Fatalf("wheel under the box: scroll %d, top %d", m.scroll, m.suggTop)
	}
	last := m.sugg[n-1]
	m.Update(tea.MouseClickMsg{X: 10, Y: top + suggRows, Button: tea.MouseLeft})
	if want := search.Complete("skin:", last); m.input.text != want || m.focus != focusQuery {
		t.Fatalf("click: %q, want %q", m.input.text, want)
	}
	m.input.set("")
	typeText(m, "skin:")
	for range 3 {
		m.Update(tea.MouseWheelMsg{X: 20, Y: top + 2, Button: tea.MouseWheelDown})
	}
	third := m.sugg[3]
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if want := search.Complete("skin:", third); m.input.text != want {
		t.Fatalf("tab after the wheel: %q, want %q", m.input.text, want)
	}
}

func TestNavClickReleasesSearch(t *testing.T) {
	m := colourModel(t, Options{})
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	typeText(m, "sk")
	if m.focus != focusQuery || len(m.sugg) == 0 {
		t.Fatalf("focus %v, %d suggestions", m.focus, len(m.sugg))
	}
	for _, it := range m.navLayout() {
		if it.sec == secNetwork {
			m.Update(tea.MouseClickMsg{X: it.x0 + 1, Y: 1, Button: tea.MouseLeft})
		}
	}
	if m.section != secNetwork || m.focus != focusMain || m.sugg != nil {
		t.Fatalf("section %s, focus %v, %d suggestions", sectionNames[m.section], m.focus, len(m.sugg))
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.input.text != "sk" || !quits(cmd) {
		t.Errorf("q went to the query: %q", m.input.text)
	}
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if quits(c) {
				return true
			}
		}
	}
	return false
}

func TestArtToggle(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	header := func() string { return strings.TrimRight(plainView(m)[m.headRows()], " │┃") }
	footer := func() string { v := plainView(m); return v[len(v)-1] }
	if !strings.HasSuffix(header(), "›‹") || !strings.Contains(footer(), "[-] COLLAPSE") || strings.Contains(header(), "HEAD") {
		t.Fatalf("big art: %q / %q", header(), footer())
	}
	m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	if !strings.HasSuffix(header(), "‹›") || !strings.Contains(footer(), "[+] EXPAND") || !strings.Contains(header(), "HEAD") {
		t.Fatalf("collapsed: %q / %q", header(), footer())
	}
	row := plainView(m)[m.headRows()]
	x := len([]rune(row[:strings.Index(row, "‹›")]))
	m.Update(tea.MouseClickMsg{X: x + 1, Y: m.headRows(), Button: tea.MouseLeft})
	if !strings.HasSuffix(header(), "›‹") || m.card {
		t.Fatalf("click: %q, card %v", header(), m.card)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.artSize = artAuto
	if !strings.HasSuffix(header(), "‹›") {
		t.Fatalf("narrow: %q", header())
	}
	m.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	if !strings.HasSuffix(header(), "›‹") {
		t.Fatalf("expanded when narrow: %q", header())
	}
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	if _, ok := m.artToggle(); ok || strings.ContainsAny(header(), "‹›") {
		t.Fatalf("no preview, still a toggle: %q", header())
	}
}

func TestClickLeavesSearch(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	typeText(m, "skin:")
	row := m.headRows() + suggRows + 4
	m.Update(tea.MouseClickMsg{X: 8, Y: row, Button: tea.MouseLeft})
	if m.focus != focusMain || m.sugg != nil || m.input.text != "skin:" {
		t.Fatalf("click on a row: focus %v, %d suggestions, %q", m.focus, len(m.sugg), m.input.text)
	}
	sel := m.sel
	m.Update(tea.MouseClickMsg{X: 8, Y: row, Button: tea.MouseLeft})
	if !m.card || m.sel != sel || m.focus != focusMain {
		t.Fatalf("second click: card %v, sel %d (was %d), focus %v", m.card, m.sel, sel, m.focus)
	}
	m.card = false
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, it := range m.navLayout() {
		if it.sec == secZilkroad {
			m.Update(tea.MouseClickMsg{X: it.x0 + 1, Y: 1, Button: tea.MouseLeft})
		}
	}
	if m.focus != focusMain || m.sugg != nil {
		t.Fatalf("active tab click: focus %v, %d suggestions", m.focus, len(m.sugg))
	}
}

func TestTicksPauseOnBlur(t *testing.T) {
	m := colourModel(t, Options{})
	m.Update(tea.BlurMsg{})
	if m.onPageTick() != nil || m.ticking || m.onShimmerTick() != nil || m.shimmerTicking {
		t.Fatalf("still ticking while blurred: page %v, shimmer %v", m.ticking, m.shimmerTicking)
	}
	m.Update(tea.FocusMsg{})
	if !m.ticking || !m.shimmerTicking {
		t.Fatalf("not restarted on focus: page %v, shimmer %v", m.ticking, m.shimmerTicking)
	}
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if m.onShimmerTick(); m.shimmerTicking {
		t.Fatal("the shimmer keeps running in the market view")
	}
}

func TestPreviewShowsAllArt(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	for _, size := range [][2]int{{120, 40}, {160, 48}, {300, 90}, {90, 30}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, expand := range []bool{true, false} {
			m.resizeArt(expand)
			m.sel = m.byID[97]
			w, h := m.resultsW(), m.bodyH()
			_, tableW, previewW, big := m.listLayout(w, h)
			if previewW == 0 {
				continue
			}
			want := m.previewThumb(m.results[m.sel])
			if big {
				want = m.previewArt(m.results[m.sel])
			}
			out := m.listLines(w, h)
			for y, l := range out {
				if l.w > w {
					t.Fatalf("%v big %v: line %d is %d wide, more than %d", size, big, y, l.w, w)
				}
			}
			for k, a := range want {
				if x := tableW + max(2, (w-tableW-previewW)/2); k == 0 && x+previewW > w-2 {
					a = truncate(line{a, previewW}, w-2-x).s
				}
				if !strings.Contains(out[k].s, a) {
					t.Fatalf("%v big %v: art line %d is missing", size, big, k)
				}
			}
		}
	}
}

func TestPanelClickBelowTheList(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	top := m.headRows()
	for _, y := range []int{1, 2} {
		m.Update(tea.MouseClickMsg{X: 5, Y: top + m.panel.listH(m) + y, Button: tea.MouseLeft})
		if m.input.text != "" {
			t.Fatalf("a click %d below the list set %q", y, m.input.text)
		}
	}
}

func TestQueryFlagIsCleaned(t *testing.T) {
	m := colourModel(t, Options{Query: " skele\n\trim\x1b[31m "})
	if m.input.text != "skele rim[31m" {
		t.Fatalf("query %q", m.input.text)
	}
}

func TestEnterSearchesBareWords(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	enter := func(typed string, down int) {
		m.focus, m.card = focusMain, false
		m.input.set("")
		m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		typeText(m, typed)
		for range down {
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	typeText(m, "rim")
	if v := plainView(m); !strings.Contains(v[len(v)-1], "[ENTER] DONE  [TAB] COMPLETE") {
		t.Fatalf("footer for a bare word: %q", v[len(v)-1])
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cases := []struct {
		typed string
		down  int
		want  string
		focus focus
	}{
		{"rim", 0, "rim", focusMain},
		{"ski", 0, "skin:", focusQuery},
		{"skin:", 0, `skin:Earthtone `, focusQuery},
		{"", 0, "lineage:", focusQuery},
	}
	for _, c := range cases {
		enter(c.typed, c.down)
		if m.input.text != c.want || m.focus != c.focus {
			t.Errorf("%q: %q, focus %v", c.typed, m.input.text, m.focus)
		}
	}
	enter("rim", 1)
	if !strings.HasPrefix(m.input.text, "eyes:") || m.focus != focusQuery {
		t.Errorf("rim after a pick: %q", m.input.text)
	}
	enter("4", 0)
	if !m.card || m.tokens[m.results[m.sel]].ID != 4 {
		t.Errorf("4 + enter: card %v", m.card)
	}
}

func TestArtCornerIsBlank(t *testing.T) {
	m := colourModel(t, Options{})
	for _, tk := range m.tokens {
		for _, p := range [][2]int{{24, 0}, {25, 0}, {24, 1}, {25, 1}} {
			if tk.Art.Base(p[0], p[1]) != (art.RGB{}) {
				t.Errorf("#%d has a pixel at %v, under the art size icon", tk.ID, p)
			}
		}
	}
}

func TestPrintCard(t *testing.T) {
	for _, c := range []struct{ cols, w int }{{0, 103}, {120, 103}, {103, 103}, {102, 56}, {80, 56}} {
		m := colourModel(t, Options{})
		lines, ok := m.PrintCard(c.cols, 7724)
		if !ok {
			t.Fatal("no card for #7724")
		}
		plain := strings.Join(lines, "\n")
		for strings.Contains(plain, "\x1b[") {
			i := strings.Index(plain, "\x1b[")
			j := i + 2
			for j < len(plain) && (plain[j] < 0x40 || plain[j] > 0x7e) {
				j++
			}
			plain = plain[:i] + plain[j+1:]
		}
		for _, want := range []string{"zkSNARK #7724_", "Floor in ZEC", "MARKET_", "prices as of"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%d columns: missing %q", c.cols, want)
			}
		}
		for _, unwanted := range []string{"[Z]", "[H]", "↗", "TRAITS_"} {
			if strings.Contains(plain, unwanted) {
				t.Errorf("%d columns: has %q", c.cols, unwanted)
			}
		}
		rows := strings.Split(plain, "\n")
		for i, r := range rows {
			if width(r) != c.w {
				t.Fatalf("%d columns: row %d is %d wide, want %d", c.cols, i, width(r), c.w)
			}
		}
		head := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "zkSNARK #7724_") })
		if stacked := head > art.CardH; stacked != (c.w == 56) {
			t.Errorf("%d columns: title on row %d, want it %s the art", c.cols, head, map[bool]string{true: "under", false: "beside"}[c.w == 56])
		}
		if len(m.keys) != 0 {
			t.Errorf("%d columns: printing left %d click areas", c.cols, len(m.keys))
		}
		if _, ok := m.PrintCard(c.cols, 10001); ok {
			t.Error("a card for #10001")
		}
	}
}

func TestPrintBids(t *testing.T) {
	snap := &market.Snapshot{Tokens: []market.Token{
		{ID: 6991, Listed: true, Ask: "0.479", Bid: "0.38"},
		{ID: 9615, Listed: true, Ask: "1", Bid: "0.8"},
		{ID: 5519, Bid: "0.23"},
		{ID: 3283, Listed: true, Ask: "0.87", Bid: "0.512"},
	}}
	rate := &market.Rate{ZecUsd: 1000}
	for _, c := range []struct{ cols, per int }{{120, 4}, {90, 2}, {60, 2}, {0, 2}} {
		lines := colourModel(t, Options{}).PrintBids(c.cols, snap, rate)
		rows := strings.Split(sgr.ReplaceAllString(strings.Join(lines, "\n"), ""), "\n")
		plain := strings.Join(rows, "\n")
		for _, want := range []string{"BIDS_", "https://zilkroad.com/bids", "ITEMS WITH BIDS:    4", "HIGHEST BID:        0.80 ZEC   ≈ $800", "AVERAGE OPEN BID:   0.481 ZEC  ≈ $481", "TOTAL OPEN VALUE:   1.922 ZEC  ≈ $1,922", "ASKING:  1.00 ZEC ≈ $1,000", "STATUS:  NOT FOR SALE", "RANK:"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%d columns: missing %q", c.cols, want)
			}
		}
		if strings.Contains(plain, "prices as of") {
			t.Errorf("%d columns: live prices marked as old", c.cols)
		}
		var order []int
		for _, id := range []string{"#9615_", "#3283_", "#6991_", "#5519_"} {
			for y, r := range rows {
				if strings.Contains(r, id) {
					order = append(order, y)
				}
			}
		}
		if len(order) != 4 || !slices.IsSorted(order) || order[c.per-1] != order[0] || c.per < 4 && order[c.per] == order[0] {
			t.Errorf("%d columns: titles on rows %v, want %d a row by highest bid", c.cols, order, c.per)
		}
		for i, r := range rows {
			if width(r) != width(rows[0]) || c.cols > 0 && width(r) > c.cols {
				t.Fatalf("%d columns: row %d is %d wide, the first %d", c.cols, i, width(r), width(rows[0]))
			}
		}
	}
	many := &market.Snapshot{}
	for id := 1; id <= 10; id++ {
		many.Tokens = append(many.Tokens, market.Token{ID: id, Bid: market.Decimal(strconv.Itoa(id))})
	}
	plain := sgr.ReplaceAllString(strings.Join(colourModel(t, Options{}).PrintBids(120, many), "\n"), "")
	if !strings.Contains(plain, "ITEMS WITH BIDS:    10") || !strings.Contains(plain, "TOTAL OPEN VALUE:   55.00 ZEC") {
		t.Errorf("the stats do not count all ten bids:\n%s", plain)
	}
	for id := 1; id <= 10; id++ {
		if shown := strings.Contains(plain, "#"+strconv.Itoa(id)+"_"); shown != (id > 1) {
			t.Errorf("#%d shown %v, want the nine highest bids", id, shown)
		}
	}
	plain = sgr.ReplaceAllString(strings.Join(colourModel(t, Options{}).PrintBids(100), "\n"), "")
	if !strings.Contains(plain, "No active bids.") || !strings.Contains(plain, "prices as of") {
		t.Errorf("without bids:\n%s", plain)
	}
}

func TestPrintSales(t *testing.T) {
	now := time.Now()
	var sales []market.Sale
	for n := range 12 {
		sales = append(sales, market.Sale{ID: 100 + n, Price: "0.3", Time: now.Add(-time.Duration(n+1) * time.Hour).Format(time.RFC3339)})
	}
	sales[0], sales[11] = sales[11], sales[0]
	sales = append(sales, market.Sale{ID: 10001, Price: "1", Time: now.Format(time.RFC3339)}, market.Sale{ID: 99, Price: "", Time: now.Format(time.RFC3339)})
	stats := &market.Stats{Floor: "0.3797", MarketCap: "3540.3228", Sales24h: 12, SalesTotal: 5622, Volume24h: "4.44278", VolumeTotal: "6639.99395299"}
	lines := colourModel(t, Options{}).PrintSales(120, sales, stats, &market.Rate{ZecUsd: 1000})
	rows := strings.Split(sgr.ReplaceAllString(strings.Join(lines, "\n"), ""), "\n")
	plain := strings.Join(rows, "\n")
	for _, want := range []string{"SALES_", "Last 9 completed zkSNARK trades on Zilkroad.", "https://zilkroad.com/sales", "FLOOR PRICE:    0.3797 ZEC", "MARKET CAP:     3,540 ZEC", "≈ $3,540,323", "24H SALES:      12", "TOTAL SALES:    5,622", "24H VOLUME:     4.443 ZEC", "TOTAL VOLUME:   6,640 ZEC", "PRICE:   0.30 ZEC ≈ $300", "SOLD:    1h ago"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(plain, "prices as of") || strings.Contains(plain, "#99_") || strings.Contains(plain, "#10001") {
		t.Errorf("a stale note or a broken sale:\n%s", plain)
	}
	var stat []string
	statW := 0
	for _, r := range rows[5:12] {
		f := strings.Fields(r)
		stat = append(stat, strings.Join(f[:min(len(f), 2)], " "))
		statW = max(statW, width(strings.TrimSpace(r)))
	}
	if want := []string{"24H SALES:", "24H VOLUME:", "FLOOR PRICE:", "", "TOTAL SALES:", "TOTAL VOLUME:", "MARKET CAP:"}; !slices.Equal(stat[:3], want[:3]) || !slices.Equal(stat[4:], want[4:]) || strings.TrimSpace(rows[8]) != strings.Repeat("-", statW) {
		t.Errorf("stats %q, want the day, a rule as wide as the stats, then the totals", stat)
	}
	var order []int
	for n := range 12 {
		for y, r := range rows {
			if strings.Contains(r, "#"+strconv.Itoa(100+n)+"_") {
				order = append(order, y)
				if n >= 9 {
					t.Errorf("#%d shown, only the nine newest should be", 100+n)
				}
			}
		}
	}
	if len(order) != 9 || !slices.IsSorted(order) || order[2] != order[0] || order[3] == order[0] {
		t.Errorf("titles on rows %v, want nine newest first, three a row", order)
	}
	plain = sgr.ReplaceAllString(strings.Join(colourModel(t, Options{}).PrintSales(120, nil), "\n"), "")
	if !strings.Contains(plain, "No sales yet.") || strings.Contains(plain, "FLOOR PRICE") {
		t.Errorf("without sales or stats:\n%s", plain)
	}
}

func TestTraitArrowFilters(t *testing.T) {
	m := colourModel(t, Options{})
	openZilkroad(m, false)
	m.sel = 11
	id := m.tokens[m.results[m.sel]].ID
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.card {
		t.Fatal("card not open")
	}
	var x, y int
	for row, l := range plainView(m) {
		if strings.Contains(l, "LINEAGE") && strings.Contains(l, "SKIN") {
			rs := []rune(l)
			for i := len(rs) - 1; i >= 0; i-- {
				if rs[i] == '↗' {
					x, y = i, row
					break
				}
			}
		}
	}
	if y == 0 {
		t.Fatal("no arrow on screen")
	}
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	skin := m.tokens[m.byID[id]].Traits[1]
	if m.card || m.input.text != (search.Suggestion{Key: 1, Value: skin}).Text() || m.tokens[m.results[m.sel]].ID != id {
		t.Fatalf("after the click: card %v, query %q, selected #%d (want #%d)", m.card, m.input.text, m.tokens[m.results[m.sel]].ID, id)
	}
	for _, r := range m.results {
		if m.tokens[r].Traits[1] != skin {
			t.Fatalf("#%d has skin %q", m.tokens[r].ID, m.tokens[r].Traits[1])
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.input.text != "" {
		t.Fatalf("esc left %q", m.input.text)
	}
}
