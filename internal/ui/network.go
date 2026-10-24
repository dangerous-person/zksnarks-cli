package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

// The network page: its line and heading like the luminaries', then every spotlighted
// collector under each other, the zkSNARK they hold on the left and their words on the
// right. The arrows pick one, and its interview plays in a window over the page.
const (
	networkIntro = "The collectors holding zkSNARKs, spotlighted one at a time."
	networkTextW = 64 // the widest the words run, so their lines read well
	// networkURL is the site's network page, where its button applies for a spotlight
	networkURL = "https://zksnarks.net/network"
)

// networkHeading is the page's title; a window too narrow for it gets plain "THE NETWORK_".
var networkHeading = blockFont{
	glyphs: []string{
		"_____._.___.__  ._______    .______  .____________._         ___ ._______  .______  .____/\\ ",
		"\\__ _:|:   |  \\ : .____/    :      \\ : .____/\\__ _:|.___    |   |: .___  \\ : __   \\ :   /  \\",
		"  |  :||   :   || : _/\\     |       || : _/\\   |  :|:   | /\\|   || :   |  ||  \\____||.  ___/",
		"  |   ||   .   ||   /  \\    |   |   ||   /  \\  |   ||   |/  :   ||     :  ||   :  \\ |     \\ ",
		"  |   ||___|   ||_.: __/    |___|   ||_.: __/  |   ||   /       | \\_. ___/ |   |___\\|      \\",
		"  |___|    |___|   :/           |___|   :/     |___||______/|___|   :/     |___|    |___\\  /",
		"                                                            :       :                    \\/ ",
		"                                                            :                               ",
	},
	tones: []string{
		"32211223322112  33221111    3322112  332211113221122         123 33211111  3322111  2211233 ",
		"332 1213   1  3 3 111122    3      3 3 111122332 1213321    1   33 1112  1 3 11   2 2   2  2",
		"  2  112   1   22 1 111     2       22 1 111   2  113   1 121   22 1   2  12  11111111  2111",
		"  1   12   1   11   1  2    2   1   11   1  2  1   12   11  2   11     1  21   1  2 1     1 ",
		"  1   233221   22211 233    33221   22211 233  1   22   1       2 123 1233 2   112332      1",
		"  11233    22233   12           22233   12     112333322111111233   22     33221    33212  2",
		"                                                            2       2                    33 ",
		"                                                            2                               ",
	},
}

// netTarget is a place on the network page a click acts on: spotlight spot's watch key
// ("w") or the rest of it, portrait and words ("").
type netTarget struct {
	rect
	spot int
	key  string
}

func (m *Model) networkKey(k string) tea.Cmd {
	spots := m.opts.Site.Spotlights
	if len(spots) == 0 {
		return nil
	}
	if m.watchOpen {
		return m.playerKey(k)
	}
	page := max(1, m.bodyH()-2)
	switch k {
	case "up", "k":
		m.selectSpot(m.spot - 1)
	case "down", "j":
		m.selectSpot(m.spot + 1)
	case "home", "g":
		m.selectSpot(0)
	case "end", "G":
		m.selectSpot(len(spots) - 1)
	case "pgup", "ctrl+b", "ctrl+u":
		m.netTop -= page
	case "pgdown", "ctrl+f", "ctrl+d", "space":
		m.netTop += page
	case "w", "enter":
		return m.openPlayer()
	case "o":
		return m.jump(spots[m.spot].Snark)
	case "a":
		return openURL(networkURL)
	}
	return nil
}

// selectSpot picks spotlight i and scrolls the page to it.
func (m *Model) selectSpot(i int) {
	m.spot = max(0, min(i, len(m.opts.Site.Spotlights)-1))
	m.netFollow = true
}

// openPlayer plays the picked spotlight's interview in its window.
func (m *Model) openPlayer() tea.Cmd {
	if m.interviewOf(m.spot) == nil {
		return nil
	}
	m.watchOpen = true
	cmd, _ := m.watchKey("w")
	return cmd
}

// closePlayer stops the interview and closes its window.
func (m *Model) closePlayer() {
	m.stopWatch()
	m.watchOpen = false
}

// playerKey handles the keys of the interview's window: p and space pause, resume or play
// again, s stops, w plays and esc closes.
func (m *Model) playerKey(k string) tea.Cmd {
	switch k {
	case "esc", "backspace":
		m.closePlayer()
		return nil
	case "space":
		k = "p"
	}
	if state, _ := m.watching(m.spot); k == "p" && state == watchStopped {
		k = "w"
	}
	cmd, _ := m.watchKey(k)
	return cmd
}

// playerOpen reports whether the interview's window is over the network page.
func (m *Model) playerOpen() bool { return m.section == secNetwork && m.watchOpen }

// networkClick picks the spotlight clicked and plays its interview when it has one; with the
// window open, a click on the video pauses or resumes it and one outside closes the window.
func (m *Model) networkClick(ms tea.Mouse) tea.Cmd {
	if m.watchOpen {
		switch {
		case !m.box.contains(ms.X, ms.Y):
			m.closePlayer()
		case m.screen.contains(ms.X, ms.Y):
			return m.playerKey("p")
		}
		return nil
	}
	for _, t := range m.netTargets {
		if !t.contains(ms.X, ms.Y) {
			continue
		}
		m.spot = t.spot
		return m.openPlayer()
	}
	return nil
}

// networkLines draws the page from line netTop and records what clicks on it act on, with a
// scrollbar test when it does not fit.
func (m *Model) networkLines(w, h int) ([]line, func(int) bool) {
	doc, head := m.pageHead(networkIntro, networkHeading, "THE NETWORK_", w)
	spots := m.opts.Site.Spotlights
	tx := art.ThumbW + luminaryGap // the words' column
	tw := max(1, min(networkTextW, w-tx))
	var targets []netTarget // on doc lines, before scrolling
	var spans [][2]int      // each spotlight's first and last line
	for i, s := range spots {
		var thumb []string
		top := 0 // the words start level with the top of the figure
		if idx, ok := m.byID[s.Snark]; ok {
			thumb = m.thumb(idx)
			top = figureTop(backdrop(m.tokens[idx].Art))
		}
		words, keys := m.spotWords(i, s, tw)
		y0 := len(doc)
		n := max(len(thumb), top+len(words))
		for y := 0; y < n; y++ {
			var r row
			if y < len(thumb) {
				r.cells(thumb[y], art.ThumbW)
			}
			if k := y - top; k >= 0 && k < len(words) {
				if k == 0 && i == m.spot {
					r.padTo(tx - 2)
					r.put(fg(m.theme.Accent), pointer)
				}
				r.padTo(tx)
				r.cells(words[k].s, words[k].w)
			}
			doc = append(doc, r.line())
		}
		for _, k := range keys { // the keys first: a click takes the first target it is in
			k.y0 += y0 + top
			k.y1 += y0 + top
			k.x0 += tx
			k.x1 += tx
			targets = append(targets, netTarget{k.rect, i, k.key})
		}
		targets = append(targets, netTarget{rect{0, y0, tx + tw, y0 + n}, i, ""})
		spans = append(spans, [2]int{y0, y0 + n})
		doc = append(doc, line{}, line{})
	}
	doc = m.drips(&m.netDrips, networkHeading, 12).draw(doc, head, &m.theme)
	doc = append(doc, line{}) // room under the last one
	if m.netFollow && m.spot < len(spans) {
		if sp := spans[m.spot]; sp[0] < m.netTop {
			m.netTop = sp[0]
		} else if sp[1] > m.netTop+h {
			m.netTop = sp[1] - h
		}
		m.netFollow = false
	}
	m.netTop = max(0, min(m.netTop, len(doc)-h))
	m.netTargets = m.netTargets[:0]
	for _, t := range targets { // to the screen: the frame's border and padding, the header
		t.x0, t.x1 = t.x0+2, t.x1+2
		t.y0, t.y1 = t.y0-m.netTop+m.headRows(), t.y1-m.netTop+m.headRows()
		m.netTargets = append(m.netTargets, t)
	}
	out := make([]line, h)
	copy(out, doc[m.netTop:])
	if len(doc) <= h {
		return out, nil
	}
	size := max(1, h*h/len(doc))
	pos := m.netTop * (h - size) / max(1, len(doc)-h)
	return out, func(i int) bool { return i >= pos && i < pos+size }
}

// spotWords is spotlight i's words in tw cells: name and handle as on the luminaries page,
// the token and the interview's length, the quote, the bio and, for the picked one with an
// interview, its watch key, with where that is for a click. The key's line is kept for all,
// so picking another does not move the page.
func (m *Model) spotWords(i int, s site.Spotlight, tw int) ([]line, []keyRect) {
	dim := fg(m.theme.Dim)
	out := []line{text(fg(m.theme.Text), clip(s.Name, tw)), text(dim, clip("@"+s.Handle, tw))}
	meta := "zkSNARK #" + itoa(s.Snark)
	if s.Runtime != "" {
		meta += " · interview " + s.Runtime
	}
	out = append(out, text(dim, clip(meta, tw)), line{})
	for _, ln := range wrap(plain("“"+s.Quote+"”"), tw) {
		var r row
		spanLine(&r, ln, "\x1b[3m"+fg(m.theme.Text), "", 1<<30)
		out = append(out, r.line())
	}
	out = append(out, line{})
	for _, ln := range wrap(plain(s.Bio), tw) {
		var r row
		spanLine(&r, ln, dim, "", 1<<30)
		out = append(out, r.line())
	}
	out = append(out, line{})
	if i != m.spot || m.interviewOf(i) == nil {
		return append(out, line{}), nil
	}
	l := m.hints("enter", "watch")
	return append(out, l), []keyRect{{rect{0, len(out), l.w, len(out) + 1}, "w"}}
}

// figureTop is the first cell row with some of the figure in it, half-blocks being two
// pixels high.
func figureTop(back []bool) int {
	for i, b := range back {
		if !b {
			return i / art.Grid / 2
		}
	}
	return 0
}

// drawPlayer layers the interview's window over the dimmed page like the card's: the
// collector's name on the top of its frame, the video on black with its progress under it,
// and on the bottom of the frame the keys on the left and the time on the right. A window
// too small for the video gets a note how big it must be.
func (m *Model) drawPlayer(cv *canvas) {
	c := m.interviewOf(m.spot)
	if c == nil {
		return
	}
	s := m.opts.Site.Spotlights[m.spot]
	top, bottom := m.headRows(), m.h-m.footRows()
	areaW, areaH := m.w-4, bottom-top
	cv.shade(0, top, m.w, bottom, 1-m.theme.Backdrop, art.RGB{})
	cols, rows := c.video.Size()
	content := m.player(m.spot, c)
	cw := cols
	small := modalChromeW+cols > areaW || modalChromeH+len(content) > areaH
	if small {
		needW := modalChromeW + cols + 4 // and the page's border and padding
		needH := modalChromeH + len(content) + m.headRows() + 2
		if needH >= marketMinH { // the market strip shows from there
			needH += 2
		}
		cw = max(1, min(48, areaW-modalChromeW))
		content = []line{text(bold(m.theme.Text), "MAKE THE WINDOW BIGGER_"), {}}
		for _, ln := range wrap(plain("the interview needs "+itoa(needW)+" x "+itoa(needH)+", the window is "+itoa(m.w)+" x "+itoa(m.h)), cw) {
			var r row
			spanLine(&r, ln, fg(m.theme.Dim), "", 1<<30)
			content = append(content, r.line())
		}
	}
	boxW, boxH := modalChromeW+cw, modalChromeH+len(content)
	x0, y0 := 2+(areaW-boxW)/2, top+max(0, (areaH-boxH)/2)
	t := m.playerTime(m.spot, c)
	keys := m.playerKeys(m.spot)
	ax, ay := m.modalBox(cv, rect{x0, y0, x0 + boxW, y0 + boxH}, s.Name, func(w int) line {
		if keys.w+3+width(t) > w { // a dash and a space each between the keys and the time
			keys = m.hints("esc", "close")
		}
		return keys
	})
	// the time sits as far from the right corner as the keys from the left one
	if fx1 := x0 + boxW - modalMargin; keys.w+3+width(t) <= fx1-1-ax-2 {
		cv.draw(fx1-modalPadX-width(t)-2, y0+boxH-1, fg(m.theme.Dim)+" "+t+" "+reset)
	}
	m.screen = rect{}
	if !small {
		cv.fill(ax, ay, ax+cols, ay+rows, m.theme.BodyBg) // the video sits on black, like the page
		m.screen = rect{ax, ay, ax + cols, ay + rows}
	}
	for k, l := range content {
		cv.draw(ax, ay+k, l.s)
	}
}
