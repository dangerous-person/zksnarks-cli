package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

const (
	secRoot = iota
	secZilkroad
	secManifesto
	secLuminaries
	secNetwork
	secAbout
)

var sectionNames = []string{"root", "zilkroad", "manifesto", "luminaries", "network", "about"}

// marketMinH is the height from which the market strip has its own row above the hints.
const marketMinH = 30

// The frame is one box: the brand and section navigation on top, the section's content,
// the market strip at the bottom, and the key hints under the box.
//
//	╭──────────────────────────────╮
//	│ zkSNARKs_       1 zilkroad … │  nav
//	├──────────────────────────────┤
//	│ / search                     │  zilkroad only
//	├──────────────────────────────┤  zilkroad only
//	│ …                            │  body
//	├──────────────────────────────┤
//	│ floor …            net …     │  market strip (tall windows)
//	╰──────────────────────────────╯
//	 [ENTER] OPEN  …     [T] THEME     hints
//
// The whole screen is on black, the art's own background, so the art's edges show.
func (m *Model) showMarket() bool { return m.h >= marketMinH }

func (m *Model) headRows() int {
	if m.section == secZilkroad {
		return 5
	}
	return 3
}

func (m *Model) footRows() int {
	if m.showMarket() {
		return 4
	}
	return 2
}

// View draws the screen; every line is exactly m.w cells wide.
func (m *Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "zkSNARKs"
	if m.w == 0 || m.h == 0 {
		return v
	}
	if m.w < minW || m.h < minH {
		v.Content = m.tooSmall()
		return v
	}
	out, cursorX := m.frameLines()
	if m.overlay() || m.playerOpen() || m.helpShown() {
		cv := newCanvas(m.w, m.h)
		for y, l := range out {
			cv.draw(0, y, l)
		}
		switch {
		case m.helpShown():
			m.drawHelp(cv)
		case m.overlay():
			m.drawCard(cv)
		default:
			m.drawPlayer(cv)
		}
		v.Content = cv.String()
	} else {
		v.Content = strings.Join(out, "\n")
	}
	if m.focus == focusQuery && cursorX >= 0 {
		v.Cursor = tea.NewCursor(cursorX, 3)
	}
	return v
}

// frameLines draws everything but an overlaid card, and the search cursor's column.
func (m *Model) frameLines() ([]string, int) {
	out := []string{m.hedge("╭", "╮"), m.side(m.navLine(), false, true), m.separator()}
	cursorX := -1
	if m.section == secZilkroad {
		ql, cx := m.queryLine()
		out = append(out, m.side(ql, false, false), m.separator())
		cursorX = cx
	}
	main, bar := m.body()
	body := main
	skip := 0
	if m.section == secZilkroad {
		body = append(m.suggestionBox(), main...)
		skip = m.suggestionLines()
	}
	n := m.h - m.footRows() - len(out)
	for i := range n {
		var l line
		if i < len(body) {
			l = body[i]
		}
		thumb := bar != nil && i < len(body) && i >= skip && bar(i-skip)
		out = append(out, m.side(l, thumb, i == n-1 && !m.showMarket()))
	}
	out = out[:m.h-m.footRows()]
	if m.showMarket() {
		out = append(out, m.separator(), m.side(m.marketRow(), false, true))
	}
	out = append(out, m.hedge("╰", "╯"), m.hintsRow())
	for i, l := range out {
		out[i] = onBg(l, m.theme.BodyBg)
	}
	return out, cursorX
}

func (m *Model) hedge(left, right string) string {
	var r row
	r.put(fg(m.theme.CornerTip), left)
	r.put(fg(m.theme.Corner), "─")
	r.put(fg(m.theme.Frame), strings.Repeat("─", max(0, m.w-4)))
	r.put(fg(m.theme.Corner), "─")
	r.put(fg(m.theme.CornerTip), right)
	return r.String()
}

func (m *Model) bodyH() int {
	h := m.h - m.headRows() - m.footRows()
	if m.section == secZilkroad {
		h -= m.suggestionLines()
	}
	return max(1, h)
}

// body returns the current section's content and, when it scrolls, a scrollbar test.
func (m *Model) body() ([]line, func(int) bool) {
	h := m.bodyH()
	switch m.section {
	case secRoot:
		if m.mont.open {
			return m.montageLines(m.innerW(), h), nil
		}
		return m.rootLines(m.innerW(), h), nil
	case secManifesto:
		return m.manifestoLines(m.innerW(), h)
	case secLuminaries:
		return m.luminaryLines(m.innerW(), h)
	case secNetwork:
		return m.networkLines(m.innerW(), h)
	case secAbout:
		return m.contactLines(m.innerW(), h), nil
	}
	return m.zilkroadBody(h)
}

type navItem struct {
	x0, x1 int // columns on screen, end exclusive
	sec    int
	text   string
}

func (m *Model) navLayout() []navItem {
	w := m.innerW()
	name := func(i int, full bool) string {
		if full || i == m.section {
			return " [" + itoa(i) + "] " + strings.ToUpper(sectionNames[i]) + " "
		}
		return " [" + itoa(i) + "] "
	}
	for _, full := range []bool{true, false} {
		total := 0
		for i := range sectionNames {
			total += width(name(i, full))
		}
		if width(brand)+2+total > w && full {
			continue
		}
		x := 2 + w - total
		var items []navItem
		for i := range sectionNames {
			l := name(i, full)
			items = append(items, navItem{x, x + width(l), i, l})
			x += width(l)
		}
		return items
	}
	return nil
}

const brand = "zkSNARKs_"

func (m *Model) navLine() line {
	items := m.navLayout()
	var r row
	r.put(bold(m.theme.Brand), brand)
	if len(items) > 0 {
		r.padTo(items[0].x0 - 2)
	}
	for _, it := range items {
		if it.sec == m.section {
			m.activeTab(&r, it.text)
			continue
		}
		key, label, _ := strings.Cut(strings.TrimSpace(it.text), " ")
		r.space(1)
		m.keyIn(&r, strings.Trim(key, "[]"), m.theme.NavKey, m.theme.HintBracket)
		if label != "" {
			r.space(1)
			r.put(fg(m.theme.NavLabel), label)
		}
		r.space(1)
	}
	return r.line()
}

const navInside = 2

func (m *Model) activeTab(r *row, text string) {
	white := art.RGB{255, 255, 255}
	key, label, _ := strings.Cut(strings.TrimSpace(text), " ")
	r.space(1)
	m.keyIn(r, strings.Trim(key, "[]"), white, m.theme.Brand)
	if label != "" {
		r.space(1)
		letters := []rune(label)
		for i, c := range letters {
			d := min(i, len(letters)-1-i, navInside)
			r.put(fg(mix(white, m.theme.NavLabel, float64(d)/(navInside+1))), string(c))
		}
	}
	r.space(1)
}

// chip draws bold text on a coloured background.
func chip(bgc, fgc art.RGB, s string) string {
	return "\x1b[1m" + fg(fgc) + bg(bgc) + s + reset
}

func (m *Model) marketRow() line {
	st := m.statusLine()
	var r row
	l := m.tildes(m.marketParts(), m.innerW()-st.w-3)
	r.cells(l.s, l.w)
	r.padTo(m.innerW() - st.w)
	r.cells(st.s, st.w)
	return r.line()
}

// tildes joins parts with a quiet "~", dropping parts from the end until they fit in n cells.
func (m *Model) tildes(parts []line, n int) line {
	const sep = " ~ "
	for len(parts) > 0 {
		w := len(sep) * (len(parts) - 1)
		for _, p := range parts {
			w += p.w
		}
		if w <= n {
			break
		}
		parts = parts[:len(parts)-1]
	}
	var r row
	for i, p := range parts {
		if i > 0 {
			r.put(fg(m.theme.MarketSep), sep)
		}
		r.cells(p.s, p.w)
	}
	return r.line()
}

// hintsRow is the line under the box: key hints on the left, the theme key (or the network
// status, when the market strip is hidden) on the right.
func (m *Model) hintsRow() string {
	right := m.hints("t", "theme")
	if !m.showMarket() {
		right = m.statusLine()
	}
	room := m.w - 2
	var hint line
	for _, h := range m.footerHints() {
		if h.w+2+right.w <= room {
			hint = h
			break
		}
	}
	if hint.w == 0 {
		right = line{}
		for _, h := range m.footerHints() {
			if h.w <= room {
				hint = h
				break
			}
		}
	}
	var r row
	r.space(1)
	r.cells(hint.s, hint.w)
	if right.w > 0 {
		r.padTo(m.w - 1 - right.w)
		r.cells(right.s, right.w)
	}
	r.padTo(m.w)
	return r.String()
}

// footerHints returns the hints for the current state, longest first.
func (m *Model) footerHints() []line {
	switch {
	case m.focus == focusQuery:
		return []line{m.hints("enter", "done", "tab", "complete", "↑↓", "pick", "esc", "cancel"), m.hints("enter", "done", "tab", "complete", "esc", "cancel")}
	case m.helpShown():
		return []line{m.hints("/", "search", "esc", "close")}
	case m.focus == focusFilters:
		return []line{m.hints("space", "toggle", "esc", "close")}
	case m.section == secRoot && m.mont.open:
		return []line{m.montageKeys()}
	case m.section == secRoot:
		var watch []string
		if m.hasMontage() {
			watch = []string{"enter", "play"}
		}
		return []line{m.hints(watch...)}
	case m.section == secManifesto && m.reading:
		return []line{m.hints("↑↓", "scroll", "space", "page", "esc", "back"), m.hints("↑↓", "scroll", "esc", "back")}
	case m.section == secManifesto:
		return []line{m.hints("enter", "read")}
	case m.section == secLuminaries:
		return []line{m.hints("↑↓", "scroll")}
	case m.section == secAbout:
		return m.contactHints()
	case m.section == secNetwork && m.watchOpen:
		return []line{m.playerKeys(m.spot)}
	case m.section == secNetwork:
		if s, ok := m.spotlight(); ok {
			var watch []string
			if m.interviewOf(m.spot) != nil {
				watch = []string{"enter", "play"}
			}
			return []line{
				m.hints(append(watch, "↑↓", "pick", "o", "open #"+itoa(s.Snark), "a", "apply")...),
				m.hints(append(watch, "↑↓", "pick", "o", "open", "a", "apply")...),
				m.hints(append(watch, "o", "open", "a", "apply")...),
			}
		}
		return []line{m.hints("a", "apply")}
	}
	other := "list"
	if m.mode == modeList {
		other = "grid"
	}
	return []line{
		m.hints("enter", "open", "f", "filters", "v", other, "?", "help"),
		m.hints("f", "filters", "?", "help"),
	}
}
