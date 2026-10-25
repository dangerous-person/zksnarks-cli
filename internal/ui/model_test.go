package ui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
