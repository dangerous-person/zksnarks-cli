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
func (m *Model) side(l line, thumb bool) string {
	l = truncate(l, m.innerW())
	var r row
	r.put(fg(m.theme.Border), "│")
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
		r.put(fg(m.theme.Border), "│")
	}
	return r.String()
}

// sideFaded is a side-bordered line next to a corner, so both side borders fade.
func (m *Model) sideFaded(l line) string {
	l = truncate(l, m.innerW())
	var r row
	r.put(fg(m.theme.Corner), "│")
	r.space(1)
	r.cells(l.s, l.w)
	r.padTo(m.w - 2)
	r.space(1)
	r.put(fg(m.theme.Corner), "│")
	return r.String()
}

func (m *Model) separator() string {
	var r row
	r.put(fg(m.theme.Border), "├"+strings.Repeat("─", m.w-2)+"┤")
	return r.String()
}

// queryLine is the search box with the result count; it also returns the cursor column.
func (m *Model) queryLine() (line, int) {
	w := m.innerW()
	right := comma(len(m.results)) + " of " + comma(m.supplyShown()) + " · " + sortLabel(m.q.Sort)
	rStyle := fg(m.theme.Dim)
	if len(m.q.Errs) > 0 {
		right, rStyle = m.q.Errs[0], fg(m.theme.Warn)
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
		r.put(fg(m.theme.Dim), clip("skele · rim cobalt · #7724 · price<1 · sort:rank", avail))
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
	r.padTo(w - width(right))
	r.put(rStyle, right)
	return r.line(), cursor
}

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
	return len(m.sugg) + 2
}

// suggestionBox lists completions under the search box while a term is being typed.
func (m *Model) suggestionBox() []line {
	if len(m.sugg) == 0 {
		return nil
	}
	const w = 44
	b := fg(m.theme.Border)
	var out []line
	out = append(out, text(b, "  ╭"+strings.Repeat("─", w-2)+"╮"))
	for i, s := range m.sugg {
		var r row
		r.put(b, "  │ ")
		cur := "  "
		if i == m.suggSel {
			cur = pointer + " "
		}
		r.put(fg(m.theme.Accent), cur)
		if s.Jump != 0 {
			r.put(fg(m.theme.Text), "jump to #"+itoa(s.Jump))
		} else {
			name := strings.ToLower(searchKey(s.Key))
			r.put(fg(m.theme.Dim), name+strings.Repeat(" ", max(1, 10-width(name))))
			r.put(fg(m.theme.Text), clip(s.Value, w-24))
			r.padTo(2 + w - 2 - 6)
			r.put(fg(m.theme.Dim), padLeft(comma(s.Count), 5))
		}
		r.padTo(2 + w - 1)
		r.put(b, "│")
		out = append(out, r.line())
	}
	var r row
	r.put(b, "  ╰─ ")
	r.put(fg(m.theme.Accent), "[tab]")
	r.put(fg(m.theme.Dim), " complete ")
	r.put(b, strings.Repeat("─", w-r.w+1)+"╯")
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

func (m *Model) helpLines(w, h int) []line {
	keys := [][2]string{
		{"GO", "1 zilkroad · 2 luminaries · 3 network"},
		{"MOVE", "hjkl or arrows · PgUp PgDn · Home End · wheel"},
		{"OPEN", "enter card · esc back · ←→ prev/next in results"},
		{"FIND", "/ search · tab completes · f filters panel"},
		{"VIEW", "v grid or list · t theme"},
		{"DATA", "r refresh prices · H history (1 request) · z open on zilkroad"},
		{"WATCH", "enter watch a spotlight interview · p pause · s stop · esc close"},
		{"APPLY", "a apply for a spotlight on zksnarks.net"},
		{"QUIT", "q · ctrl+c"},
		{"", ""},
		{"QUERY", "skele rim · eyes:\"rim cobalt\" · lineage:noise,dune"},
		{"", "#7724 · listed · purged · apparition · auction"},
		{"", "price<1.5 · rank<=100 · last>2"},
		{"", "sort:price · sort:-price · sort:rank · sort:new"},
	}
	bw := 0
	for _, k := range keys {
		bw = max(bw, 8+width(k[1]))
	}
	bw = min(bw+4, w)
	var box []line
	b := fg(m.theme.Border)
	box = append(box, text(b, "╭─ keys "+strings.Repeat("─", max(0, bw-9))+"╮"))
	for _, k := range keys {
		var r row
		r.put(b, "│ ")
		r.put(bold(m.theme.Accent), k[0])
		r.padTo(10)
		r.put(fg(m.theme.Text), clip(k[1], bw-12))
		r.padTo(bw - 1)
		r.put(b, "│")
		box = append(box, r.line())
	}
	box = append(box, text(b, "╰"+strings.Repeat("─", max(0, bw-2))+"╯"))
	out := make([]line, h)
	top := max(0, (h-len(box))/2)
	left := max(0, (w-bw)/2)
	for i, l := range box {
		if top+i >= h {
			break
		}
		var r row
		r.space(left)
		r.cells(l.s, l.w)
		out[top+i] = r.line()
	}
	return out
}
