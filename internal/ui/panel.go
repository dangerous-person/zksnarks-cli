package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

const panelW = 30

type prow struct {
	heading string // a group title; not selectable
	status  string // a status row
	key     int    // a trait value row
	value   string
}

// panel is the filters side panel: every status and trait value with a checkbox. Toggling
// a row edits the query text, so the query and the panel always agree.
type panel struct {
	open   bool
	cursor int
	scroll int
	find   input
	rows   []prow
}

var statusRows = []string{"listed", "apparition", "auction", "purged"}

func (m *Model) openPanel() {
	m.panel.open = true
	m.focus = focusFilters
	m.panel.build(m)
	m.ensureVisible()
}

func (p *panel) build(m *Model) {
	p.rows = p.rows[:0]
	find := strings.ToLower(strings.TrimSpace(p.find.text))
	if find == "" {
		p.rows = append(p.rows, prow{heading: "STATUS"})
		for _, s := range statusRows {
			p.rows = append(p.rows, prow{status: s})
		}
	}
	for k := 0; k < 5; k++ {
		var vals []prow
		for _, v := range m.ix.Cat.Values[k] {
			if find == "" || strings.Contains(strings.ToLower(v), find) {
				vals = append(vals, prow{key: k, value: v})
			}
		}
		if len(vals) > 0 {
			p.rows = append(p.rows, prow{heading: strings.ToUpper(atlas.TraitNames[k])})
			p.rows = append(p.rows, vals...)
		}
	}
	p.cursor = min(p.cursor, len(p.rows)-1)
	p.skipHeading(m, 1)
}

func (p *panel) skipHeading(m *Model, dir int) {
	for p.cursor >= 0 && p.cursor < len(p.rows) && p.rows[p.cursor].heading != "" {
		p.cursor += dir
	}
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		p.cursor = 0
		for p.cursor < len(p.rows) && p.rows[p.cursor].heading != "" {
			p.cursor++
		}
	}
	p.clamp(m)
}

func (p *panel) listH(m *Model) int { return max(1, m.bodyH()-4) }

func (p *panel) clamp(m *Model) {
	h := p.listH(m)
	if p.cursor < p.scroll {
		p.scroll = p.cursor
		if p.scroll > 0 && p.rows[p.scroll-1].heading != "" {
			p.scroll-- // keep the group title in view
		}
	}
	if p.cursor >= p.scroll+h {
		p.scroll = p.cursor - h + 1
	}
	p.scroll = max(0, min(p.scroll, len(p.rows)-h))
}

func (m *Model) panelKey(msg tea.KeyPressMsg, k string) tea.Cmd {
	p := &m.panel
	switch k {
	case "esc":
		if p.find.text != "" {
			p.find.set("")
			p.build(m)
			return nil
		}
		p.open = false
		m.focus = focusMain
		m.ensureVisible()
	case "tab":
		m.focus = focusMain // keep the panel open and move around the results
	case "up", "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
			p.skipHeading(m, -1)
		}
	case "down", "ctrl+n":
		if p.cursor < len(p.rows)-1 {
			p.cursor++
			p.skipHeading(m, 1)
		}
	case "pgup":
		p.cursor = max(0, p.cursor-p.listH(m))
		p.skipHeading(m, 1)
	case "pgdown":
		p.cursor = min(len(p.rows)-1, p.cursor+p.listH(m))
		p.skipHeading(m, -1)
	case "space", "enter":
		m.toggleRow(p.cursor)
	case "ctrl+c":
		return tea.Quit
	default:
		if p.find.key(msg) {
			p.cursor = 0
			p.build(m)
		}
	}
	return nil
}

func (m *Model) toggleRow(i int) {
	if i < 0 || i >= len(m.panel.rows) {
		return
	}
	r := m.panel.rows[i]
	switch {
	case r.heading != "":
		return
	case r.status != "":
		m.input.set(search.ToggleStatus(m.q, r.status))
	default:
		m.input.set(search.ToggleValue(m.q, r.key, r.value))
	}
	m.refresh(true)
}

func (p *panel) wheel(m *Model, b tea.MouseButton) {
	switch b {
	case tea.MouseWheelUp:
		p.scroll = max(0, p.scroll-2)
	case tea.MouseWheelDown:
		p.scroll = max(0, min(p.scroll+2, len(p.rows)-p.listH(m)))
	}
}

func (p *panel) click(m *Model, y int) {
	i := p.scroll + y - 2
	if y < 2 || i >= len(p.rows) || p.rows[i].heading != "" {
		return
	}
	p.cursor = i
	m.focus = focusFilters
	m.toggleRow(i)
}

func (m *Model) ensureFacets() {
	if m.facets == nil {
		f := m.ix.Facets(m.q, m.tokens)
		m.facets = &f
		m.statusCnt = m.ix.StatusCounts(m.q, m.tokens)
	}
}

// lines draws the panel: its find box, the rows and a detail line for the cursor row.
func (p *panel) lines(m *Model, h int) []line {
	m.ensureFacets()
	b := fg(m.theme.Border)
	if m.focus == focusFilters {
		b = fg(m.theme.Accent)
	}
	inner := panelW - 2
	var out []line
	var top row
	top.put(b, "╭─ ")
	top.put(bold(m.theme.Text), "filters")
	top.put(b, " "+strings.Repeat("─", panelW-top.w-2)+"╮")
	out = append(out, top.line())
	var find row
	find.put(b, "│")
	if p.find.text == "" {
		find.put(fg(m.theme.Dim), " type to find a value")
	} else {
		find.put(fg(m.theme.Accent), " / ")
		find.put(fg(m.theme.Text), clip(p.find.text, inner-4))
	}
	find.padTo(panelW - 1)
	find.put(b, "│")
	out = append(out, find.line())
	listH := p.listH(m)
	for y := 0; y < listH; y++ {
		var r row
		r.put(b, "│")
		i := p.scroll + y
		if i < len(p.rows) {
			p.drawRow(m, &r, i, inner)
		}
		r.padTo(panelW - 1)
		r.put(b, "│")
		out = append(out, r.line())
	}
	var d row
	d.put(b, "│")
	d.put(fg(m.theme.Dim), " "+clip(p.detail(m), inner-1))
	d.padTo(panelW - 1)
	d.put(b, "│")
	out = append(out, d.line())
	out = append(out, text(b, "╰"+strings.Repeat("─", panelW-2)+"╯"))
	for len(out) < h {
		out = append(out, line{})
	}
	return out[:h]
}

func (p *panel) drawRow(m *Model, r *row, i, inner int) {
	pr := p.rows[i]
	if pr.heading != "" {
		r.put(bold(m.theme.Dim), " "+pr.heading)
		return
	}
	cur := " "
	if i == p.cursor && m.focus == focusFilters {
		cur = pointer
	}
	r.put(fg(m.theme.Accent), cur)
	var checked bool
	var name string
	var n int
	if pr.status != "" {
		checked, name, n = m.q.HasStatus(pr.status), pr.status, m.statusCnt[pr.status]
	} else {
		checked, name, n = m.q.HasValue(pr.key, pr.value), pr.value, m.facets[pr.key][pr.value]
	}
	box, bs := "[ ]", fg(m.theme.Dim)
	if checked {
		box, bs = "[x]", fg(m.theme.Accent)
	}
	r.put(bs, box)
	r.space(1)
	ns := fg(m.theme.Text)
	if n == 0 && !checked {
		ns = fg(m.theme.Dim)
	}
	r.put(ns, clip(name, 16))
	cnt := comma(n)
	r.padTo(1 + inner - width(cnt) - 1)
	r.put(fg(m.theme.Dim), cnt)
}

// detail describes the cursor row: share of the live supply and the cheapest listing.
func (p *panel) detail(m *Model) string {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return ""
	}
	pr := p.rows[p.cursor]
	switch {
	case pr.heading != "":
		return ""
	case pr.status != "":
		return map[string]string{
			"listed":     "has an ask price",
			"apparition": "one of the apparitions",
			"auction":    "on auction now",
			"purged":     "purged, hidden by default",
		}[pr.status]
	}
	s := percent(m.ix.Cat.Counts[pr.key][pr.value], m.opts.Atlas.Meta.Supply) + " of supply"
	if f, ok := m.floors[pr.key][pr.value]; ok {
		s += " · floor " + zecFloat(f)
	}
	return s
}
