package ui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
)

func colourModel(t *testing.T, o Options) *Model {
	t.Helper()
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	content, err := embedded.LoadSite()
	if err != nil {
		t.Fatal(err)
	}
	o.Atlas, o.Site, o.Client = a, content, market.New(true)
	m := New(o)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
	return m
}

var trueColourRe = regexp.MustCompile(`\x1b\[[0-9;]*?[34]8;2;(\d+);(\d+);(\d+)[0-9;]*m`)

func trueColours(s string) []art.RGB {
	var out []art.RGB
	for _, g := range trueColourRe.FindAllStringSubmatch(s, -1) {
		var c art.RGB
		for i := range c {
			v, _ := strconv.Atoi(g[i+1])
			c[i] = uint8(v)
		}
		out = append(out, c)
	}
	return out
}

func TestNoColorGreysTheChrome(t *testing.T) {
	m := colourModel(t, Options{NoColor: true, State: State{Theme: "zec"}})
	openZilkroad(m, true)
	if m.theme.Name != "plain" {
		t.Fatalf("theme %q", m.theme.Name)
	}
	lines := plainView(m)
	if last := lines[len(lines)-1]; strings.Contains(last, "THEME") {
		t.Errorf("hints offer the theme: %q", last)
	}
	tinted := false
	for _, c := range trueColours(m.View().Content) {
		tinted = tinted || c[0] != c[1] || c[1] != c[2]
	}
	if !tinted {
		t.Error("the art lost its colours")
	}
	m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if m.theme.Name != "plain" {
		t.Errorf("t switched to %q", m.theme.Name)
	}
	for _, sec := range []int{secRoot, secManifesto} {
		m.setSection(sec)
		for _, c := range trueColours(m.View().Content) {
			if c[0] != c[1] || c[1] != c[2] {
				t.Errorf("section %s: chrome colour %v", sectionNames[sec], c)
				break
			}
		}
	}
	if got := m.SavedState().Theme; got != "zec" {
		t.Errorf("saved theme %q, want zec", got)
	}
}

func TestThemeKeyWithColour(t *testing.T) {
	m := colourModel(t, Options{State: State{Theme: "zec"}})
	m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if m.theme.Name != "phosphor" || m.SavedState().Theme != "phosphor" {
		t.Errorf("theme %q, saved %q", m.theme.Name, m.SavedState().Theme)
	}
}

func TestIndexedView(t *testing.T) {
	for _, mode := range []string{"gallery", "list"} {
		m := colourModel(t, Options{Color: colorprofile.ANSI256})
		openZilkroad(m, mode == "gallery")
		s := m.View().Content
		if strings.Contains(s, "8;2;") || !strings.Contains(s, "\x1b[38;5;") || !strings.Contains(s, "\x1b[48;5;") {
			t.Errorf("%s: not drawn in indexed colours", mode)
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if s := m.View().Content; !m.overlay() || strings.Contains(s, "8;2;") {
			t.Errorf("%s: card open %v, not drawn in indexed colours", mode, m.overlay())
		}
	}
	m := colourModel(t, Options{Color: colorprofile.TrueColor})
	openZilkroad(m, true)
	if s := m.View().Content; strings.Contains(s, "8;5;") || !strings.Contains(s, "\x1b[48;2;") {
		t.Error("truecolor view is not drawn in truecolor")
	}
}

func TestSwatchIsTheSkin(t *testing.T) {
	m := colourModel(t, Options{})
	red := func(c art.RGB) bool { return int(c[0]) > int(c[1])+60 && int(c[0]) > int(c[2])+60 }
	green := func(c art.RGB) bool { return int(c[1]) > int(c[0])+20 && int(c[1]) > int(c[2])+20 }
	light := func(c art.RGB) bool { return int(c[0])*3+int(c[1])*6+int(c[2]) > 1000 }
	for _, c := range []struct {
		id   int
		want func(art.RGB) bool
	}{
		{1943, red},
		{1952, red},
		{1963, red},
		{1955, green},
		{1946, green},
		{144, light},
		{5, light},
		{340, green},
		{1132, green},
	} {
		if got := m.swatch(m.byID[c.id]); !c.want(got) {
			t.Errorf("#%d %v: swatch %v", c.id, m.tokens[m.byID[c.id]].Traits[:4], got)
		}
	}
}
