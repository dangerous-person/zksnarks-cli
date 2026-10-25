package ui

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

const rootLogo = `.______ .____/\ .________.______  .______  .______  .____/\ .________
\____  |:   /  \|    ___/:      \ :      \ : __   \ :   /  \|    ___/
/  ____||.  ___/|___    \|       ||   .   ||  \____||.  ___/|___    \
\      ||     \ |       /|   |   ||   :   ||   :  \ |     \ |       /
 \__:__||      \|__:___/ |___|   ||___|   ||   |___\|      \|__:___/
    :   |___\  /   :         |___|    |___||___|    |___\  /   :
    ∙        \/                                          \/`

const rootShades = `0112210 1122100 0011222220011221  0011222  0011222  1122100 001122222
01222  01   1  10    22220      0 0      2 0 22   1 1   1  10    2222
1  2222122  12221122    21       11   2   21  22222222  12221122    2
2      12     2 2       11   2   22   1   22   2  1 2     2 2       1
 22211001      222212100 00112   122110   11   221001      222212100
    1   00121  1   1         11100    1210000112    00121  1   1
    1        00                                          00`

var rootFont = func() blockFont {
	var f blockFont
	shades := strings.Split(rootShades, "\n")
	tones := strings.NewReplacer("0", "3", "1", "2", "2", "1")
	for i, l := range strings.Split(rootLogo, "\n") {
		pad := strings.Repeat(" ", logoW-width(l))
		f.glyphs = append(f.glyphs, l+pad)
		f.tones = append(f.tones, tones.Replace(shades[i])+pad)
	}
	return f
}()

const rootIntro = "A collection of shielded identities stored on Zcash."

const (
	logoW    = 69
	stillGap = 2
)

func (m *Model) rootLines(w, h int) []line {
	m.rootWatch = rect{}
	if w < logoW || h < m.rootMinH() {
		return m.rootTooSmall(w, h)
	}
	logo := append(m.rootLogoLines(), line{})
	logo = m.drips(&m.rootDrips, rootFont, 13).draw(logo, 0, &m.theme)
	words, intro := m.underStill(), m.rootIntroLines()
	still, sw := m.frontStill()
	if sw > w || len(logo)+len(still)+stillGap+len(words) > h {
		still, sw = nil, 0
	}
	if len(intro)+len(logo)+len(still)+stillGap*boolInt(still != nil)+len(words) > h {
		intro = nil
	}
	logo = append(intro, logo...)
	if still != nil {
		still = append(still, make([]line, stillGap)...)
	}
	pw := max(logoW, sw)
	out := make([]line, h)
	top, left := (h-len(logo)-len(still)-len(words))/2, (w-pw)/2
	y := top
	for _, b := range []struct {
		lines []line
		w     int
	}{{logo, logoW}, {still, sw}, {words, logoW}} {
		for _, l := range b.lines {
			var r row
			r.space(left + (pw-b.w)/2)
			r.cells(l.s, l.w)
			out[y] = r.line()
			y++
		}
	}
	if still != nil {
		m.drawPieces(out, left+(pw-sw)/2, top+len(logo))
	}
	if m.hasMontage() {
		l := m.watchLine()
		x := 2 + left + (pw-logoW)/2 + (logoW-l.w)/2
		m.rootWatch = rect{x, m.headRows() + y - 1, x + l.w, m.headRows() + y}
	}
	return out
}

func (m *Model) rootMinH() int { return len(rootFont.glyphs) + 1 + len(m.underStill()) }

func (m *Model) rootIntroLines() []line {
	var out []line
	for _, ln := range wrap(plain(rootIntro), logoW) {
		var r row
		spanLine(&r, ln, fg(m.theme.Dim), fg(m.theme.Dim), 1<<30)
		out = append(out, centreIn(r.line(), logoW))
	}
	return append(out, line{})
}

func (m *Model) watchLine() line { return m.pageKey("enter", "play") }

func (m *Model) pageKey(key, label string) line {
	var r row
	if key == "enter" {
		r.put(fg(m.theme.HintBracket), "[")
		m.shimmerKey(&r, key, art.RGB{255, 255, 255}, m.theme.HintKey)
		r.put(fg(m.theme.HintBracket), "]")
	} else {
		m.keyHint(&r, key)
	}
	r.space(1)
	r.put(fg(m.theme.HintLabel), strings.ToUpper(label))
	return r.line()
}

const (
	shimmerStep   = 40 * time.Millisecond
	shimmerSteps  = 24
	shimmerPause  = 2 * time.Second
	shimmerSpread = 2.0
)

type shimmerTickMsg struct{}

func (m *Model) shimmers() bool { return true }

func (m *Model) startShimmer() tea.Cmd {
	if m.shimmerTicking || !m.shimmers() {
		return nil
	}
	m.shimmerTicking, m.shimmerAt = true, -1
	return shimmerTick(shimmerPause)
}

func shimmerTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return shimmerTickMsg{} })
}

func (m *Model) onShimmerTick() tea.Cmd {
	if !m.shimmers() {
		m.shimmerTicking, m.shimmerAt = false, -1
		return nil
	}
	if m.shimmerAt++; m.shimmerAt >= shimmerSteps {
		m.shimmerAt = -1
		return shimmerTick(shimmerPause)
	}
	return shimmerTick(shimmerStep)
}

func (m *Model) shimmerKey(r *row, key string, rest, peak art.RGB) {
	letters := []rune(strings.ToUpper(key))
	x := math.Inf(-1)
	if m.shimmerAt >= 0 {
		x = -shimmerSpread + float64(m.shimmerAt)/(shimmerSteps-1)*(float64(len(letters)-1)+2*shimmerSpread)
	}
	for i, c := range letters {
		k := max(0, 1-math.Abs(float64(i)-x)/shimmerSpread)
		r.put(fg(mix(rest, peak, k)), string(c))
	}
}

func (m *Model) rootLogoLines() []line {
	var out []line
	shades := strings.Split(rootShades, "\n")
	for i, l := range strings.Split(rootLogo, "\n") {
		var r row
		for j, c := range []rune(l) {
			r.put(m.shade(shades[i], j), string(c))
		}
		out = append(out, r.line())
	}
	return out
}

func (m *Model) underStill() []line {
	if !m.hasMontage() {
		return nil
	}
	return []line{centreIn(m.watchLine(), logoW)}
}

const (
	stillStep   = 440 * time.Millisecond
	stillEvery  = 2
	fragLong    = 10
	fragRoom    = 2
	fragDrift   = 5
	fragsAtOnce = 3
)

type still struct {
	frame   *canvas
	tape    [][]rune
	frags   []frag
	pieces  []piece
	growing []growth
	rng     *rand.Rand
	tick    int
	next    int
}

type frag struct{ x0, x1, y, room int }

type growth struct{ frag, grown, wait int }

type piece struct {
	x, y, flown, to int
	rs              []rune
	colours         []art.RGB
}

func (m *Model) frontStill() ([]line, int) {
	s := m.stillNow()
	if s == nil {
		return nil, 0
	}
	cv := &canvas{w: s.frame.w, h: s.frame.h, c: slices.Clone(s.frame.c)}
	shift := s.tick / stillEvery
	for y, tape := range s.tape {
		for x := range cv.w {
			if c := cv.at(x, y); c.r != ' ' {
				c.r = tape[(x+shift)%cv.w]
			}
		}
	}
	for _, g := range s.growing {
		f := s.frags[g.frag]
		for x := f.x0; x < f.x1-g.grown; x++ {
			c := cv.at(x, f.y)
			c.r, c.hasBg = ' ', false
		}
	}
	var out []line
	for _, l := range strings.Split(cv.String(), "\n") {
		out = append(out, line{l, cv.w})
	}
	return out, cv.w
}

func (m *Model) stillNow() *still {
	if !m.frontRead {
		m.frontRead = true
		m.front = m.readStill("front.zkv")
	}
	return m.front
}

func (s *still) step() {
	s.tick++
	keep := s.pieces[:0]
	for _, p := range s.pieces {
		p.x--
		p.flown++
		if p.flown >= p.to || !s.frame.at(p.x, p.y).empty() {
			continue
		}
		keep = append(keep, p)
	}
	s.pieces = keep
	grow := s.growing[:0]
	for _, g := range s.growing {
		if g.wait > 0 {
			g.wait--
		} else {
			g.grown++
		}
		if f := s.frags[g.frag]; g.grown < f.x1-f.x0 {
			grow = append(grow, g)
		}
	}
	s.growing = grow
	if s.tick >= s.next {
		s.loosen()
	}
}

func (s *still) loosen() {
	if len(s.frags) == 0 || len(s.growing) >= fragsAtOnce {
		return
	}
	i := s.rng.IntN(len(s.frags))
	for _, g := range s.growing {
		if g.frag == i {
			return
		}
	}
	f, shift := s.frags[i], s.tick/stillEvery
	p := piece{x: f.x0, y: f.y, to: fragRoom + s.rng.IntN(f.room-fragRoom+1)}
	for x := f.x0; x < f.x1; x++ {
		p.rs = append(p.rs, s.tape[f.y][(x+shift)%s.frame.w])
		p.colours = append(p.colours, s.frame.at(x, f.y).fg)
	}
	s.pieces = append(s.pieces, p)
	s.growing = append(s.growing, growth{frag: i, wait: 2})
	s.next = s.tick + 3 + s.rng.IntN(5)
}

func (m *Model) drawPieces(out []line, x0, y0 int) {
	for _, p := range m.front.pieces {
		y := y0 + p.y
		if y < 0 || y >= len(out) {
			continue
		}
		dark := 0.15 + 0.85*float64(p.flown)/float64(p.to)
		for i, r := range p.rs {
			if x := x0 + p.x + i; x >= 0 {
				out[y] = putCell(out[y], x, fg(mix(p.colours[i], m.theme.BodyBg, dark))+string(r)+reset)
			}
		}
	}
}

type stillTickMsg struct{}

func (m *Model) startStill() tea.Cmd {
	if m.stillTicking || m.stillNow() == nil {
		return nil
	}
	m.stillTicking = true
	return m.stillTick()
}

func (m *Model) stillTick() tea.Cmd {
	return tea.Tick(stillStep/stillEvery, func(time.Time) tea.Msg { return stillTickMsg{} })
}

func (m *Model) onStillTick() tea.Cmd {
	if m.section != secRoot {
		m.stillTicking = false
		return nil
	}
	if !m.mont.open {
		m.front.step()
	}
	return m.stillTick()
}

func (m *Model) readStill(name string) *still {
	b, err := readMedia(m.opts.Media, name)
	if err != nil {
		return nil
	}
	v, err := video.Read(b)
	if err != nil {
		return nil
	}
	cols, rows := v.Size()
	s := &still{frame: newCanvas(cols, rows), rng: rand.New(rand.NewPCG(uint64(rows), uint64(cols)))}
	for y, l := range v.Frame(0, m.opts.Glyph) {
		s.frame.draw(0, y, l)
	}
	rng := rand.New(rand.NewPCG(uint64(cols), uint64(rows)))
	for y := range rows {
		var chars []rune
		for x := range cols {
			if r := s.frame.at(x, y).r; r != ' ' {
				chars = append(chars, r)
			}
		}
		tape := make([]rune, cols)
		for x := range cols {
			switch r := s.frame.at(x, y).r; {
			case r != ' ':
				tape[x] = r
			case len(chars) > 0:
				tape[x] = chars[rng.IntN(len(chars))]
			default:
				tape[x] = ' '
			}
		}
		s.tape = append(s.tape, tape)
		s.frags = append(s.frags, s.lineFrags(y)...)
	}
	return s
}

func (s *still) lineFrags(y int) []frag {
	var runs [][2]int
	for x := 0; x < s.frame.w; {
		if s.frame.at(x, y).empty() {
			x++
			continue
		}
		x0 := x
		for x < s.frame.w && !s.frame.at(x, y).empty() {
			x++
		}
		runs = append(runs, [2]int{x0, x})
	}
	longest := 0
	for _, r := range runs {
		longest = max(longest, r[1]-r[0])
	}
	var out []frag
	for _, r := range runs {
		n := r[1] - r[0]
		if r[0] >= s.frame.w/2 || n > fragLong || n >= longest {
			continue
		}
		room := 0
		for room < fragDrift && s.frame.at(r[0]-room-1, y).empty() {
			room++
		}
		if room >= fragRoom {
			out = append(out, frag{r[0], r[1], y, room})
		}
	}
	return out
}

func (m *Model) rootKey(k string) tea.Cmd {
	if m.mont.open {
		return m.montageKey(k)
	}
	if k == "enter" {
		return m.openMontage()
	}
	return nil
}

func (m *Model) rootClick(ms tea.Mouse) tea.Cmd {
	if m.mont.open {
		return m.montageClick(ms)
	}
	if m.rootWatch.contains(ms.X, ms.Y) {
		return m.openMontage()
	}
	return nil
}

func (m *Model) shade(shades string, j int) string {
	k := 2
	if j < len(shades) && shades[j] >= '0' && shades[j] <= '2' {
		k = int(shades[j] - '0')
	}
	return fg(scale(m.theme.Text, []float64{1, 2.0 / 3, 1.0 / 3}[k]))
}

func (m *Model) rootTooSmall(w, h int) []line {
	lines := []line{
		text(bold(m.theme.Text), "MAKE THE WINDOW BIGGER_"),
		{},
		text(fg(m.theme.Dim), clip("The front page needs "+itoa(logoW+m.w-w)+" x "+itoa(m.rootMinH()+m.h-h)+".", w)),
		text(fg(m.theme.Dim), clip("This window is "+itoa(m.w)+" x "+itoa(m.h)+".", w)),
	}
	for i := range lines {
		lines[i] = centreIn(lines[i], w)
	}
	return centre(lines, 0, w, h)
}
