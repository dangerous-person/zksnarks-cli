package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

const (
	helpW    = 64
	helpCol  = 31
	helpSide = 17
)

var helpFind = [][2]string{
	{"a trait, by first letters", "skele"},
	{"more traits, all must match", "skele rim"},
	{"one trait, exact value", `eyes:"rim cobalt"`},
	{"one trait, this or that", "lineage:noise,dune"},
	{"one zkSNARK, or a few", "#7724   #7724 #12"},
}

var helpShowSort = [][4]string{
	{"for sale now", "listed", "cheapest first", "sort:price"},
	{"under 1.5 ZEC", "price<1.5", "priciest first", "sort:-price"},
	{"last sold over 2", "last>2", "rarest first", "sort:rank"},
	{"the 100 rarest", "rank<=100", "newest listings", "sort:new"},
	{"burnt", "purged", "by number", "sort:number"},
}

const helpStatuses = "unlisted  apparition  auction"

var helpOther = [][2]string{
	{"other statuses", helpStatuses},
	{"mix any of these", "skele sort:rank"},
}

func (m *Model) helpShown() bool { return m.section == secZilkroad && m.helpOpen }

func (m *Model) helpLines() []line {
	dim, bright := fg(m.theme.Heading), fg(mix(m.theme.Heading, m.theme.Text, 0.5))
	heading := func(s string) line { return text(bold(m.theme.Text), s) }
	pair := func(desc, example string) line {
		var r row
		r.put(dim, desc)
		r.padTo(helpCol)
		r.put(bright, example)
		return r.line()
	}
	twin := func(ld, le, rd, re string) line {
		var r row
		r.put(dim, ld)
		r.padTo(helpSide + 1)
		r.put(bright, le)
		r.padTo(helpCol)
		r.put(dim, rd)
		r.padTo(helpCol + helpSide)
		r.put(bright, re)
		return r.line()
	}
	rule := text(fg(m.theme.TraitBorder), strings.Repeat("-", helpW))

	var intro row
	intro.put(dim, "Press ")
	m.keyHint(&intro, "/")
	intro.put(dim, " and type.")

	var sections row
	sections.put(bold(m.theme.Text), "SHOW ONLY_")
	sections.padTo(helpCol)
	sections.put(bold(m.theme.Text), "SORT_")

	out := []line{intro.line(), {}, heading("FIND_")}
	for _, p := range helpFind {
		out = append(out, pair(p[0], p[1]))
	}
	out = append(out, rule, sections.line())
	for _, t := range helpShowSort {
		out = append(out, twin(t[0], t[1], t[2], t[3]))
	}
	out = append(out, rule, heading("OTHER_"))
	for _, p := range helpOther {
		out = append(out, pair(p[0], p[1]))
	}
	return out
}

func (m *Model) drawHelp(cv *canvas) {
	top, bottom := m.headRows(), m.h-m.footRows()
	left, right := 2, m.w-2
	areaW, areaH := right-left, bottom-top
	cv.shade(0, top, m.w, bottom, 1-m.theme.Backdrop, art.RGB{})
	lines := m.helpLines()
	contentW := min(helpW, areaW-modalChromeW)
	contentH := min(len(lines), areaH-modalChromeH)
	m.helpTop = max(0, min(m.helpTop, len(lines)-contentH))
	boxW, boxH := contentW+modalChromeW, contentH+modalChromeH
	x0 := left + (areaW-boxW)/2
	y0 := top + (areaH-boxH)/2
	scroll := contentH < len(lines)
	ax, ay := m.modalBox(cv, rect{x0, y0, x0 + boxW, y0 + boxH}, "SEARCH HELP_", func(w int) line {
		if scroll {
			return m.hints("↑↓", "scroll", "esc", "close")
		}
		return m.hints("esc", "close")
	})
	for k, l := range lines[m.helpTop : m.helpTop+contentH] {
		cv.draw(ax, ay+k, truncate(l, contentW).s)
	}
}

func (m *Model) helpKey(k string) tea.Cmd {
	switch k {
	case "esc", "backspace", "?":
		m.helpOpen = false
	case "/":
		m.helpOpen = false
		m.card = false
		m.focusQuery()
	case "up":
		m.helpTop--
	case "down":
		m.helpTop++
	}
	return nil
}
