package ui

import (
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// The luminaries page: a block-letter heading, every luminary's portrait without its
// background and with the name and handle under the figure, and a last, empty slot saying
// more are coming. It only scrolls.
const (
	luminaryIntro = "Luminaries join the registry one at a time as each plate is verified."
	luminaryGap   = 4                      // between portraits, as in the gallery
	luminaryEvery = 110 * time.Millisecond // the empty slot's animation step
	cursorTicks   = 5                      // steps per half blink of its cursor: 550 ms, a terminal's pace
)

// luminaryNext is the empty slot's text; its last character blinks like a cursor. Where
// there is room it breaks after "luminaries", as on the site.
const (
	luminaryNext  = "The next set of luminaries is being decrypted_"
	luminaryNextW = 26
)

// nextSlot is the empty slot's figure: the figure every portrait is drawn on, eyes and all.
var nextSlot = []string{
	"..........................",
	"..........................",
	"..........................",
	"..........................",
	"..........................",
	"..........................",
	".....##..##########.......",
	".......#############......",
	"...##.##############......",
	".....################.....",
	"........#############.....",
	"...#.################.....",
	"......###############.....",
	"........######oo##oo#.....",
	"....#..#######oo##oo#.....",
	".....################.....",
	"..##....#############.....",
	".........############.....",
	"......##############......",
	".........###########......",
	"....##..###########.......",
	"......##############......",
	".......##############.....",
	"...##################.....",
	".....#################....",
	"....##################....",
}

// The empty slot's figure sheds: its drawing stays as it is, and now and then one of its
// loose blocks lets go of a single pixel, which steps away to the left and vanishes before
// the art's edge.
type shedding struct {
	loose  [][3]int // the loose blocks: row, first column, length
	sparks []spark
	rng    *rand.Rand
	tick   int
}

// spark is a pixel let go on row y, now at column x, which steps left every few ticks and
// vanishes past column end.
type spark struct {
	x, y, end, every int
}

// sparkOdds lets a pixel go a tick in so many, by how many are already on their way; two at
// most.
var sparkOdds = []int{3, 10}

func newShedding() *shedding {
	const n = art.Grid
	sh := &shedding{rng: rand.New(rand.NewPCG(7, 24))}
	solid := func(x, y int) bool { return x >= 0 && y >= 0 && x < n && y < n && nextSlot[y][x] != '.' }
	// the body is what joins the eyes; the rest are the loose blocks
	body := make([]bool, n*n)
	stack := []int{13*n + 14}
	body[stack[0]] = true
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			x, y := i%n+d[0], i/n+d[1]
			if j := y*n + x; solid(x, y) && !body[j] {
				body[j] = true
				stack = append(stack, j)
			}
		}
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if solid(x, y) && !body[y*n+x] {
				k := x
				for k < n && solid(k, y) && !body[y*n+k] {
					k++
				}
				sh.loose = append(sh.loose, [3]int{y, x, k - x})
				x = k
			}
		}
	}
	return sh
}

// step moves the pixels whose turn it is and now and then lets a new one go, from a loose
// block that has none on its way.
func (sh *shedding) step() {
	sh.tick++
	keep := sh.sparks[:0]
	for _, s := range sh.sparks {
		if sh.tick%s.every == 0 {
			s.x--
		}
		if s.x >= s.end {
			keep = append(keep, s)
		}
	}
	sh.sparks = keep
	if len(sh.sparks) >= len(sparkOdds) || sh.rng.IntN(sparkOdds[len(sh.sparks)]) != 0 {
		return
	}
	b := sh.loose[sh.rng.IntN(len(sh.loose))]
	x := b[1] - 1
	for _, s := range sh.sparks {
		if s.y == b[0] {
			return
		}
	}
	if x < 0 {
		return
	}
	sh.sparks = append(sh.sparks, spark{x, b[0], max(0, x-1-sh.rng.IntN(4)), 3 + sh.rng.IntN(3)})
}

// art draws the figure and its pixels on their way in c; back is everything but the drawing,
// so the text under it keeps still.
func (sh *shedding) art(c art.RGB) (art.Art, []bool) {
	const n = art.Grid
	a := art.Art{Palette: []art.RGB{{}, c}, Pix: make([]uint8, n*n), Tex: make([]uint8, n*n), Dot: make([]uint8, n*n)}
	back := make([]bool, n*n)
	for y, r := range nextSlot {
		for x, p := range r {
			back[y*n+x] = p == '.'
			if p == '#' {
				a.Pix[y*n+x] = 1
			}
		}
	}
	for _, s := range sh.sparks {
		a.Pix[s.y*n+s.x] = 1
	}
	return a, back
}

// blockFont is a heading drawn in block letters: the glyphs row by row and each cell's tone,
// ' ' none, '1' dark, '2' mid, '3' bright and '4' bright on a mid background.
type blockFont struct {
	glyphs, tones []string
}

func (f blockFont) width() int { return width(f.glyphs[0]) }

// lines draws the heading in three tones of the theme's text: bright, mid and dark, spaced
// like a terminal's bright white, white and grey.
func (f blockFont) lines(th *Theme) []line {
	mid, dark := mix(th.Text, th.BodyBg, 0.17), mix(th.Text, th.BodyBg, 0.55)
	tone := map[byte]string{'1': fg(dark), '2': fg(mid), '3': fg(th.Text), '4': fg(th.Text) + bg(mid)}
	out := make([]line, len(f.glyphs))
	for y, g := range f.glyphs {
		var r row
		for x, c := range []rune(g) {
			r.put(tone[f.tones[y][x]], string(c))
		}
		out[y] = r.line()
	}
	return out
}

// pageHead is the top of the luminaries and network pages: their line about themselves, the
// heading in block letters (or plain text where it does not fit) and a blank line. top is
// the line the block letters start on, -1 for plain text.
func (m *Model) pageHead(intro string, f blockFont, plainHead string, w int) (lines []line, top int) {
	out := []line{{}}
	for _, ln := range wrap(plain(intro), w) {
		var r row
		spanLine(&r, ln, fg(m.theme.Dim), fg(m.theme.Dim), 1<<30)
		out = append(out, r.line())
	}
	out = append(out, line{})
	top = -1
	if f.width() <= w {
		top = len(out)
		out = append(out, f.lines(&m.theme)...)
	} else {
		out = append(out, text(bold(m.theme.Text), plainHead))
	}
	return append(out, line{}), top
}

// The headings drip: a drop leaves one of the colons at the bottom of the letters, now and
// then as two dots that run on as one, and runs down the same column a row at a time,
// greying out until it is gone; half a second to a second later the next one leaves another
// colon.
const (
	dripDepth = 3 // rows a drop runs below its colon
	dripEvery = 4 // ticks a drop takes for a row
	dripMax   = 2 // drops at once, should one outlast the wait for the next
)

type dripping struct {
	from  [][3]int // the colons drops leave: column, row and tone
	drops []drip
	rng   *rand.Rand
	tick  int
	next  int // the tick the next drop leaves at
	last  int // the colon the last one left
}

// drip is a drop dy rows under colon from, age ticks old. A double one leaves as a colon;
// light is how bright it starts: 0 the colon's own tone, 1 light grey, 2 white.
type drip struct {
	from, dy, age int
	double        bool
	light         int
}

// newDripping finds the colons at the bottom of a heading's columns.
func newDripping(f blockFont, seed uint64) *dripping {
	d := &dripping{rng: rand.New(rand.NewPCG(seed, 5)), last: -1}
	rows := make([][]rune, len(f.glyphs))
	for y, g := range f.glyphs {
		rows[y] = []rune(g)
	}
	for x := range rows[0] {
		for y := len(rows) - 1; y >= 0; y-- {
			if c := rows[y][x]; c != ' ' {
				if c == ':' {
					d.from = append(d.from, [3]int{x, y, int(f.tones[y][x])})
				}
				break
			}
		}
	}
	d.next = 4 + d.rng.IntN(8) // the first soon after the page opens
	return d
}

func (d *dripping) step() {
	d.tick++
	keep := d.drops[:0]
	for _, p := range d.drops {
		if p.age++; p.age%dripEvery == 0 {
			p.dy++
		}
		if p.dy <= dripDepth {
			keep = append(keep, p)
		}
	}
	d.drops = keep
	if len(d.from) == 0 || d.tick < d.next || len(d.drops) >= dripMax {
		return
	}
	from := d.rng.IntN(len(d.from))
	if len(d.from) > 1 && from == d.last { // another colon than the last
		from = (from + 1 + d.rng.IntN(len(d.from)-1)) % len(d.from)
	}
	light := []int{0, 0, 1, 1, 2}[d.rng.IntN(5)]
	d.drops = append(d.drops, drip{from: from, dy: 1, double: d.rng.IntN(3) == 0, light: light})
	d.last = from
	d.next = d.tick + dripDepth*dripEvery + 3 + d.rng.IntN(5) // once it has run, a moment later
}

// draw puts the drops on the page's lines, the heading starting on line top; a drop shows
// only on a cell with nothing in it.
func (d *dripping) draw(doc []line, top int, th *Theme) []line {
	if top < 0 {
		return doc
	}
	tone := map[int]art.RGB{'1': mix(th.Text, th.BodyBg, 0.55), '2': mix(th.Text, th.BodyBg, 0.17), '3': th.Text, '4': th.Text}
	for _, p := range d.drops {
		c := d.from[p.from]
		start := []art.RGB{tone[c[2]], mix(th.Text, th.BodyBg, 0.17), th.Text}[p.light]
		ch := "."
		if p.double && p.dy == 1 {
			ch = ":"
		}
		if y := top + c[1] + p.dy; y < len(doc) {
			doc[y] = putCell(doc[y], c[0], fg(mix(start, th.BodyBg, 0.22*float64(p.dy)))+ch+reset)
		}
	}
	return doc
}

// putCell writes s, one styled character, over line l at column x when the cell there shows
// nothing, padding the line to reach it.
func putCell(l line, x int, s string) line {
	cv := newCanvas(max(l.w, x+1), 1)
	cv.draw(0, 0, l.s)
	if c := cv.at(x, 0); c.r != ' ' || c.hasBg && c.bg != (art.RGB{}) {
		return l
	}
	cv.draw(x, 0, s)
	return line{cv.String(), cv.w}
}

// drips is a page's dripping, made the first time it shows.
func (m *Model) drips(d **dripping, f blockFont, seed uint64) *dripping {
	if *d == nil {
		*d = newDripping(f, seed)
	}
	return *d
}

// luminaryHeading is the page's title; a window too narrow for it gets plain "LUMINARIES_".
var luminaryHeading = blockFont{
	glyphs: []string{
		".___    .____     ._____.___ .___ .______  .______  .______  .___ ._______.________",
		"|   |   |    |___ :         |: __|:      \\ :      \\ : __   \\ : __|: .____/|    ___/",
		"|   |   |    |   ||   \\  /  || : ||       ||   .   ||  \\____|| : || : _/\\ |___    \\",
		"|   |/\\ |    :   ||   |\\/   ||   ||   |   ||   :   ||   :  \\ |   ||   /  \\|       /",
		"|   /  \\|        ||___| |   ||   ||___|   ||___|   ||   |___\\|   ||_.: __/|__:___/ ",
		"|______/|. _____/       |___||___|    |___|    |___||___|    |___|   :/      :     ",
		"         :/                                                                        ",
		"         :                                                                         ",
	},
	tones: []string{
		"3322    33221     3321112111 3321 3322112  3322111  3322111  3321 33221111332211111",
		"3   1   3    1111 3         12 1113      3 3      1 3 11   2 2 1113 1111223    1111",
		"2   1   2    1   12   2  2  11 2 12       22   1   12  1111111 2 12 1 111 2211    1",
		"1   233 1    1   11   233   11   12   1   11   2   11   1  2 1   11   1  21       2",
		"1   2  22        211111 2   21   233221   211223   22   112331   22211 23311121233 ",
		"1111123333 211233       1123311233    22233    2123333221    11233   12      2     ",
		"         22                                                                        ",
		"         1                                                                         ",
	},
}

// backdrop marks the art's background: the commonest colour on its edge and every pixel of
// that colour joined to the edge, so the same colour inside the figure stays.
func backdrop(a art.Art) []bool {
	const n = art.Grid
	count := map[uint8]int{}
	for i := 0; i < n; i++ {
		for _, p := range [][2]int{{i, 0}, {i, n - 1}, {0, i}, {n - 1, i}} {
			count[a.Pix[p[1]*n+p[0]]]++
		}
	}
	var bgi uint8
	for k, c := range count {
		if c > count[bgi] || c == count[bgi] && k < bgi {
			bgi = k
		}
	}
	back := make([]bool, n*n)
	var stack []int
	push := func(x, y int) {
		if i := y*n + x; x >= 0 && y >= 0 && x < n && y < n && !back[i] && a.Pix[i] == bgi {
			back[i] = true
			stack = append(stack, i)
		}
	}
	for i := 0; i < n; i++ {
		push(i, 0)
		push(i, n-1)
		push(0, i)
		push(n-1, i)
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := i%n, i/n
		push(x-1, y)
		push(x+1, y)
		push(x, y-1)
		push(x, y+1)
	}
	return back
}

// withoutBackdrop is the art with its background black, like the tokens' own.
func withoutBackdrop(a art.Art, back []bool) art.Art {
	out := a
	black := uint8(len(a.Palette))
	for i, c := range a.Palette {
		if c == (art.RGB{}) {
			black = uint8(i)
		}
	}
	if int(black) == len(a.Palette) {
		out.Palette = append(append([]art.RGB(nil), a.Palette...), art.RGB{})
	}
	out.Pix = append([]uint8(nil), a.Pix...)
	out.Tex = append([]uint8(nil), a.Tex...)
	for i, b := range back {
		if b {
			out.Pix[i], out.Tex[i] = black, 0
		}
	}
	return out
}

// figureX is where the figure starts on the lowest row of the art that has some of it.
func figureX(back []bool) int {
	const n = art.Grid
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			if !back[y*n+x] {
				return x
			}
		}
	}
	return 0
}

// darkBodies lists the portraits whose body is near black, drawn for a light background:
// on black it would vanish, so without the background it is lifted a little.
var darkBodies = map[string]bool{"jswihart": true}

// lighterBody lifts the figure's near-black parts that reach its outline, in their own tone;
// dark parts inside the figure, such as the eyes, stay.
func lighterBody(a art.Art, back []bool) art.Art {
	const n = art.Grid
	dark := func(i int) bool {
		c := a.Palette[a.Pix[i]]
		return !back[i] && int(c[0])+int(c[1])+int(c[2]) < 90
	}
	out := a
	out.Palette = append([]art.RGB(nil), a.Palette...)
	out.Pix = append([]uint8(nil), a.Pix...)
	lifted := map[uint8]uint8{}
	seen := make([]bool, n*n)
	for i := range seen {
		if seen[i] || !dark(i) {
			continue
		}
		comp, outside := []int{i}, false
		seen[i] = true
		for k := 0; k < len(comp); k++ {
			x, y := comp[k]%n, comp[k]/n
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= n || ny >= n || back[ny*n+nx] {
					outside = true
					continue
				}
				if j := ny*n + nx; !seen[j] && dark(j) {
					seen[j] = true
					comp = append(comp, j)
				}
			}
		}
		if !outside {
			continue
		}
		for _, j := range comp {
			p := a.Pix[j]
			if _, ok := lifted[p]; !ok && len(out.Palette) < 256 {
				c := a.Palette[p]
				var l art.RGB
				for k, base := range []int{10, 8, 6} {
					l[k] = uint8(min(255, int(c[k])*16/10+base))
				}
				out.Palette = append(out.Palette, l)
				lifted[p] = uint8(len(out.Palette) - 1)
			}
			if q, ok := lifted[p]; ok {
				out.Pix[j] = q
			}
		}
	}
	return out
}

type pageTickMsg struct{}

// startPageTick runs the animations of the luminaries and network pages while one of them
// is open: the headings' drops, and the luminaries' empty slot and its cursor. They stop on
// the next tick after the page closes.
func (m *Model) startPageTick() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking, m.lumTicks = true, 0
	return pageTick()
}

func pageTick() tea.Cmd {
	return tea.Tick(luminaryEvery, func(time.Time) tea.Msg { return pageTickMsg{} })
}

func (m *Model) onPageTick() tea.Cmd {
	switch m.section {
	case secRoot:
		m.drips(&m.rootDrips, rootFont, 13).step()
	case secLuminaries:
		m.lumTicks++
		m.shedding().step()
		m.drips(&m.lumDrips, luminaryHeading, 11).step()
	case secNetwork:
		m.drips(&m.netDrips, networkHeading, 12).step()
	case secZilkroad:
		if len(m.results) > 0 {
			m.ticking = false
			return nil
		}
		m.shedding().step()
	default:
		m.ticking = false
		return nil
	}
	return pageTick()
}

func (m *Model) shedding() *shedding {
	if m.shed == nil {
		m.shed = newShedding()
	}
	return m.shed
}

func (m *Model) luminaryKey(k string) {
	page := max(1, m.bodyH()-2)
	switch k {
	case "up", "k":
		m.lumTop--
	case "down", "j":
		m.lumTop++
	case "pgup", "ctrl+b", "ctrl+u":
		m.lumTop -= page
	case "pgdown", "ctrl+f", "ctrl+d", "space":
		m.lumTop += page
	case "home", "g":
		m.lumTop = 0
	case "end", "G":
		m.lumTop = 1 << 30
	}
}

// luminaryLines draws the page from line lumTop, with a scrollbar test when it does not fit.
func (m *Model) luminaryLines(w, h int) ([]line, func(int) bool) {
	doc, head := m.pageHead(luminaryIntro, luminaryHeading, "LUMINARIES_", w)
	// every luminary, then the empty slot
	type slot struct {
		art   art.Art
		back  []bool
		lines func(tw int) []line
	}
	var slots []slot
	dim := fg(m.theme.Dim)
	wrapped := func(s string, tw int, style string) []line {
		var out []line
		for _, ln := range wrap(plain(s), tw) {
			var r row
			spanLine(&r, ln, style, style, 1<<30)
			out = append(out, r.line())
		}
		return out
	}
	for _, l := range m.opts.Site.Luminaries {
		back := backdrop(l.Art)
		a := withoutBackdrop(l.Art, back)
		if darkBodies[l.Handle] {
			a = lighterBody(a, back)
		}
		slots = append(slots, slot{a, back, func(tw int) []line {
			return []line{text(fg(m.theme.Text), clip(l.Name, tw)), text(dim, clip("@"+l.Handle, tw))}
		}})
	}
	quiet := mix(m.theme.Dim, m.theme.BodyBg, 0.5) // the slot is half as bright as the text
	na, nb := m.shedding().art(quiet)
	slots = append(slots, slot{na, nb, func(tw int) []line {
		out := wrapped(luminaryNext, tw, fg(quiet))
		if last := &out[len(out)-1]; m.lumTicks/cursorTicks%2 == 1 {
			i := strings.LastIndex(last.s, "_")
			last.s = last.s[:i] + " " + last.s[i+1:]
		}
		return out
	}})
	pitch := art.ThumbW + luminaryGap
	cols := max(1, (w+luminaryGap)/pitch)
	for start := 0; start < len(slots); start += cols {
		group := slots[start:min(len(slots), start+cols)]
		var arts [][]string
		var notes [][]line
		tall := 0
		for i, sl := range group {
			arts = append(arts, art.Thumb(sl.art, m.opts.Glyph))
			indent := figureX(sl.back)
			tw := art.ThumbW - indent
			if start+i == len(slots)-1 { // the last slot's text may run past the portrait
				tw = min(luminaryNextW, w-i*pitch-indent)
			}
			var note []line
			for _, ln := range sl.lines(tw) {
				var r row
				r.space(indent)
				r.cells(ln.s, ln.w)
				note = append(note, r.line())
			}
			notes = append(notes, note)
			tall = max(tall, len(note))
		}
		side := func(cell func(i int) (line, bool)) line {
			var r row
			for i := range group {
				r.padTo(i * pitch)
				if l, ok := cell(i); ok {
					r.cells(l.s, l.w)
				}
			}
			return r.line()
		}
		for y := 0; y < art.ThumbH; y++ {
			doc = append(doc, side(func(i int) (line, bool) { return line{arts[i][y], art.ThumbW}, true }))
		}
		doc = append(doc, line{})
		for y := 0; y < tall; y++ {
			doc = append(doc, side(func(i int) (line, bool) {
				if y < len(notes[i]) {
					return notes[i][y], true
				}
				return line{}, false
			}))
		}
		doc = append(doc, line{}, line{})
	}
	doc = m.drips(&m.lumDrips, luminaryHeading, 11).draw(doc, head, &m.theme)
	m.lumTop = max(0, min(m.lumTop, len(doc)-h))
	out := make([]line, h)
	copy(out, doc[m.lumTop:])
	if len(doc) <= h {
		return out, nil
	}
	size := max(1, h*h/len(doc))
	pos := m.lumTop * (h - size) / max(1, len(doc)-h)
	return out, func(i int) bool { return i >= pos && i < pos+size }
}
