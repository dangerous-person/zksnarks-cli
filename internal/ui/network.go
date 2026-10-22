package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

// The network page: its line and heading like the luminaries', then every spotlighted
// collector under each other, the zkSNARK they hold on the left and their words on the
// right. The arrows pick one.
const (
	networkIntro = "The collectors holding zkSNARKs, spotlighted one at a time."
	networkTextW = 64 // the widest the words run, so their lines read well
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

// netTarget is a place on the network page a click acts on: spotlight spot's key ("o") or
// the rest of it ("").
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
	case "o", "enter":
		return m.jump(spots[m.spot].Snark)
	}
	return nil
}

// selectSpot picks spotlight i and scrolls the page to it.
func (m *Model) selectSpot(i int) {
	m.spot = max(0, min(i, len(m.opts.Site.Spotlights)-1))
	m.netFollow = true
}

// networkClick presses a spotlight's key or picks it.
func (m *Model) networkClick(ms tea.Mouse) tea.Cmd {
	for _, t := range m.netTargets {
		if !t.contains(ms.X, ms.Y) {
			continue
		}
		m.spot = t.spot
		if t.key == "o" {
			return m.jump(m.opts.Site.Spotlights[t.spot].Snark)
		}
		return nil
	}
	return nil
}

// networkLines draws the page from line netTop and records what clicks on it act on, with a
// scrollbar test when it does not fit.
func (m *Model) networkLines(w, h int) ([]line, func(int) bool) {
	doc := m.pageHead(networkIntro, networkHeading, "THE NETWORK_", w)
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
	if n := m.opts.Site.Slots - len(spots); n > 0 {
		doc = append(doc, text(fg(m.theme.Dim), clip(itoa(n)+" more spotlight slots to come", w)))
	}
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
// the token and the interview's length, the quote, the bio and the key, with where it is for
// a click.
func (m *Model) spotWords(i int, s site.Spotlight, tw int) ([]line, []keyRect) {
	dim := fg(m.theme.Dim)
	out := []line{text(fg(m.theme.Text), clip(s.Name, tw)), text(dim, clip("@"+s.Handle, tw))}
	meta := "holds #" + itoa(s.Snark)
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
	pairs := [][3]string{{"o", "open #" + itoa(s.Snark) + " in zilkroad", "open #" + itoa(s.Snark)}}
	y := len(out)
	draw := func(label int) (line, []keyRect) { // the long labels, or the short ones
		var r row
		var keys []keyRect
		for _, p := range pairs {
			if r.w > 0 {
				r.space(2)
			}
			x0 := r.w
			m.pageKey(&r, p[0], p[label])
			keys = append(keys, keyRect{rect{x0, y, r.w, y + 1}, p[0]})
		}
		return r.line(), keys
	}
	l, keys := draw(1)
	if l.w > tw {
		l, keys = draw(2)
	}
	return append(out, l), keys
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

// pageKey draws "[K] label" the way the network page shows its keys.
func (m *Model) pageKey(r *row, key, label string) {
	r.put(fg(m.theme.Accent), "["+strings.ToUpper(key)+"]")
	r.put(fg(m.theme.Dim), " "+label)
}
