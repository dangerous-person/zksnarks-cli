package ui

import (
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

func (m *Model) tooSmall() string {
	msg := "terminal too small: " + itoa(m.w) + "x" + itoa(m.h) + ", need " + itoa(minW) + "x" + itoa(minH)
	lines := make([]string, m.h)
	for i := range lines {
		var r row
		if i == m.h/2 {
			r.space((m.w - width(msg)) / 2)
			r.put(fg(m.theme.Dim), clip(msg, m.w))
		}
		r.padTo(m.w)
		lines[i] = onBg(r.String(), m.theme.BodyBg)
	}
	return strings.Join(lines, "\n")
}

// side wraps a content line in the side borders; the right one doubles as the scrollbar. A
// row that starts on the list's selection bar carries it into the padding, so the bar has a
// cell of room on both sides.
func (m *Model) side(l line, thumb, corner bool) string {
	l = truncate(l, m.innerW())
	edge := fg(m.theme.Frame)
	if corner {
		edge = fg(m.theme.Corner)
	}
	var r row
	r.put(edge, "│")
	if sel := bg(m.theme.ListSel); strings.HasPrefix(l.s, sel) {
		r.put(sel, " ")
	} else {
		r.space(1)
	}
	r.cells(l.s, l.w)
	r.padTo(m.w - 1)
	if thumb {
		r.put(fg(m.theme.Scrollbar), "┃")
	} else {
		r.put(edge, "│")
	}
	return r.String()
}

func (m *Model) separator() string {
	var r row
	r.put(fg(m.theme.Frame), "├"+strings.Repeat("─", m.w-2)+"┤")
	return r.String()
}

// queryLine is the search box with the result count; it also returns the cursor column.
func (m *Model) queryLine() (line, int) {
	w := m.innerW()
	count, rest := comma(len(m.results)), " of "+comma(m.supplyShown())+" · "+sortLabel(m.q.Sort)
	right := count + rest
	if len(m.q.Errs) > 0 {
		count, rest = "", clip(m.q.Errs[0], w/2)
		right = rest
	}
	right = clip(right, w/2)
	var r row
	m.keyIn(&r, "/", m.theme.Prompt, m.theme.HintBracket) // "[/] SEARCH:" like the key hints
	// while typing the label stays and the query scrolls; otherwise a query too long for the
	// label takes its room
	if label := " SEARCH: "; m.focus == focusQuery || width(m.input.text) < w-r.w-width(label)-width(right)-2 {
		r.put(fg(m.theme.HintLabel), label)
	} else {
		r.space(1)
	}
	avail := w - r.w - width(right) - 2
	cursor := 2 + r.w
	switch {
	case m.input.text == "" && m.focus != focusQuery:
		r.put(fg(m.theme.MarketLabel), clip(queryHint, avail))
	default:
		runes := m.input.r
		start := 0
		if len(runes) > avail-1 { // keep the cursor in view
			start = max(0, min(m.input.pos-(avail-2), len(runes)-(avail-1)))
		}
		vis := runes[start:min(len(runes), start+avail-1)]
		r.put(fg(m.theme.Input), string(vis))
		cursor += m.input.pos - start
	}
	m.clearKey = rect{}
	if clear := m.hints("esc", "clear"); m.focus == focusMain && !m.card && m.input.text != "" && r.w+2+clear.w <= w-width(right)-2 {
		r.space(2)
		x := 2 + r.w
		r.cells(clear.s, clear.w)
		m.clearKey = rect{x, 3, x + clear.w, 4}
	}
	r.padTo(w - width(right))
	switch {
	case count == "":
		r.put(fg(m.theme.Warn), right)
	case width(right) < width(count+rest):
		r.put(fg(m.theme.MarketLabel), right)
	default:
		r.put(fg(m.theme.Market), count)
		r.put(fg(m.theme.MarketLabel), rest)
	}
	return r.line(), cursor
}

const queryHint = "skele · rim cobalt · #7724 · price<1 · sort:rank"

func (m *Model) supplyShown() int {
	if m.q.HasStatus("purged") {
		return len(m.tokens)
	}
	return m.opts.Atlas.Meta.Supply
}

func sortLabel(s search.Sort) string {
	arrow := " ↑"
	if s.Desc {
		arrow = " ↓"
	}
	switch s.Field {
	case "price":
		return "price" + arrow
	case "rank":
		if s.Desc {
			return "most common"
		}
		return "rarest"
	case "new":
		if s.Desc {
			return "oldest listing"
		}
		return "newest listing"
	}
	if s.Desc {
		return "by number ↓"
	}
	return "by number"
}

func (m *Model) suggestionLines() int {
	if len(m.sugg) == 0 {
		return 0
	}
	return min(len(m.sugg), suggRows) + 2
}

func (m *Model) suggX() int {
	if m.panel.open {
		return suggX + panelW + 2
	}
	return suggX
}

func (m *Model) inSuggestions(x, y int) bool {
	top, x0 := m.headRows(), m.suggX()
	return len(m.sugg) > 0 && x >= x0 && x < x0+suggW && y >= top && y < top+m.suggestionLines()
}

func (m *Model) drawSuggestions(cv *canvas) {
	box := m.suggestionBox()
	if len(box) == 0 {
		return
	}
	x, y := m.suggX(), m.headRows()
	x1 := x + suggW
	if m.mode == modeList {
		_, tableW, _, _ := m.listLayout(m.resultsW(), m.bodyH())
		x1 = max(x1, x-2+tableW+1)
	}
	x0 := x - 2
	if !m.panel.open {
		x0 = 1
	}
	cv.fill(x0, y, x1, y+len(box), m.theme.BodyBg)
	for i, l := range box {
		cv.draw(x, y+i, l.s)
	}
}

// suggestionBox lists completions under the search box while a term is being typed.
func (m *Model) suggestionBox() []line {
	if len(m.sugg) == 0 {
		return nil
	}
	const w = suggW
	b := fg(m.theme.Border)
	n := len(m.sugg)
	size, pos := suggRows, 0
	if n > suggRows {
		size = max(1, suggRows*suggRows/n)
		pos = m.suggTop * (suggRows - size) / (n - suggRows)
	}
	parts := strings.Fields(m.input.text)
	keyed := len(parts) > 0 && strings.Contains(parts[len(parts)-1], ":")
	var out []line
	out = append(out, text(b, "╭"+strings.Repeat("─", w-2)+"╮"))
	for y := range min(n, suggRows) {
		i := m.suggTop + y
		s := m.sugg[i]
		var r row
		r.put(b, "│ ")
		cur := "  "
		if i == m.suggSel {
			cur = pointer + " "
		}
		r.put(fg(m.theme.Accent), cur)
		switch {
		case s.Jump != 0:
			r.put(fg(m.theme.Text), "jump to #"+itoa(s.Jump))
		case s.Term != "":
			r.put(fg(m.theme.Text), s.Term+strings.Repeat(" ", max(1, 13-width(s.Term))))
			if s.Count > 0 {
				r.put(fg(m.theme.Dim), clip(s.Note, w-26))
			} else {
				r.put(fg(m.theme.Dim), clip(s.Note, w-20))
			}
		case keyed:
			r.put(fg(m.theme.Text), clip(s.Value, w-13))
		default:
			name := strings.ToLower(searchKey(s.Key))
			r.put(fg(m.theme.Dim), name+strings.Repeat(" ", max(1, 13-width(name))))
			r.put(fg(m.theme.Text), clip(s.Value, w-26))
		}
		if s.Count > 0 {
			r.padTo(w - 2 - 6)
			r.put(fg(m.theme.Dim), padLeft(comma(s.Count), 5))
		}
		r.padTo(w - 1)
		if n > suggRows && y >= pos && y < pos+size {
			r.put(fg(m.theme.Scrollbar), "┃")
		} else {
			r.put(b, "│")
		}
		out = append(out, r.line())
	}
	var r row
	r.put(b, "╰─ ")
	key := m.hints("tab", "complete")
	if m.enterCompletes() {
		key = m.hints("enter", "complete")
	}
	r.cells(key.s, key.w)
	r.space(1)
	if n > suggRows {
		count := itoa(m.suggSel+1) + " of " + itoa(n)
		r.put(b, strings.Repeat("─", w-4-width(count)-r.w))
		r.put(fg(m.theme.Dim), " "+count+" ")
	}
	r.put(b, strings.Repeat("─", w-1-r.w)+"╯")
	out = append(out, r.line())
	return out
}

func padLeft(s string, n int) string {
	if width(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-width(s)) + s
}

// zilkroadBody returns the market explorer and, when it scrolls, a scrollbar test per line.
func (m *Model) zilkroadBody(h int) ([]line, func(int) bool) {
	var res []line
	if m.mode == modeList {
		res = m.listLines(m.resultsW(), h)
	} else {
		res = m.galleryLines(m.resultsW(), h)
	}
	if m.panel.open {
		pl := m.panel.lines(m, h)
		for i := range res {
			var r row
			if i < len(pl) {
				r.cells(pl[i].s, pl[i].w)
			}
			r.padTo(panelW + 2)
			r.cells(res[i].s, res[i].w)
			res[i] = r.line()
		}
	}
	return res, m.scrollbar(h)
}

func (m *Model) scrollbar(h int) func(int) bool {
	total, view := m.contentH(), m.viewH()
	if total <= view {
		return nil
	}
	off := h - view // the list header line
	size := max(1, view*view/total)
	pos := m.scroll * (view - size) / max(1, total-view)
	return func(i int) bool {
		i -= off
		return i >= pos && i < pos+size
	}
}

func searchKey(k int) string { return search.KeyName(k) }
