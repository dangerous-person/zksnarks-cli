package ui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
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
