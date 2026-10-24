package ui

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"rsc.io/qr"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

const contactAddress = "u1nsh3r8zzgw3dchskyq7t30xthu5awuksg8u63fgs6uwmlxdczgx0q4cjtr3thkjnlyl22dxtk78s3y7c8lkuxavtldruekpprvpmml7v"

const contactByline = "noisebody · Zecathon 2026"

var contactParagraphs = [][]string{
	{
		"I love moving blocks and building things, so I keep",
		"putting them back in the terminal.",
	},
	{
		"This one holds all 10,000 zkSNARKs, the zilkroad",
		"market as it moves, luminaries and holder spotlights.",
	},
	{
		"No wallet, no account, and it never asks who you are.",
		"Every picture and video here is drawn in text and comes",
		"with the app. All it fetches is public market data.",
	},
	{
		"I built it for the zkSNARKs Zecathon 2026. If something",
		"broke or made you smile, tell me in a shielded memo.",
		"Only I can read it.",
	},
}

var contactSteps = []string{
	"Scan it with a Zcash wallet, write your message in the memo",
	"and send any amount; the wallet adds a fee of about 0.0001 ZEC.",
}

var contactFigure = []string{
	"...##..##########...",
	".....#############..",
	".##.##############..",
	"...################.",
	"......#############.",
	".#.################.",
	"....###############.",
	"......######oo##oo#.",
	"..#..#######oo##oo#.",
	"...################.",
	"##....#############.",
	".......############.",
	"....##############..",
	".......###########..",
	"..##..###########...",
	"....##############..",
	".....##############.",
	".##################.",
	"...#################",
	"..##################",
}

const (
	contactGap   = 6
	contactStack = 2
)

var contactKeeps = [][]int{{0, 1, 2, 3}, {0, 3}, {3}}

func (m *Model) contactLines(w, h int) []line {
	m.contactShow, m.contactPic = rect{}, rect{}
	if m.contactOpen {
		return m.contactQRLines(w, h)
	}
	show := m.pageKey("enter", "send message")
	if v := m.contactClipNow(); v != nil {
		cols, rows := v.Size()
		words := m.contactWords(contactKeeps[0], show, w)
		ww := widest(words)
		if bh := max(rows, len(words)); cols+contactGap+ww <= w && bh <= h {
			return m.contactBlock(v, words, cols+contactGap+ww, bh, (bh-rows)/2, cols+contactGap, (bh-len(words))/2, w, h)
		}
		if bh := rows + contactStack + len(words); max(cols, ww) <= w && bh <= h {
			return m.contactBlock(v, words, max(cols, ww), bh, 0, 0, rows+contactStack, w, h)
		}
	}
	var words []line
	for _, keep := range contactKeeps {
		if words = m.contactWords(keep, show, w); len(words) <= h {
			break
		}
	}
	left := max(0, (w-widest(words))/2)
	top := max(0, (h-len(words))/2)
	x, y := 2+left, m.headRows()+top+len(words)-1
	m.contactShow = rect{x, y, x + show.w, y + 1}
	return centre(words, left, w, h)
}

func (m *Model) contactBlock(v *video.Video, words []line, bw, bh, picTop, wordsX, wordsTop, w, h int) []line {
	cols, rows := v.Size()
	pic := v.Frame(m.contactFrame, m.opts.Glyph)
	left, top := (w-bw)/2, (h-bh)/2
	out := make([]line, h)
	for y := range bh {
		var r row
		r.space(left)
		if i := y - picTop; i >= 0 && i < rows {
			r.cells(pic[i], cols)
		}
		if i := y - wordsTop; i >= 0 && i < len(words) {
			r.padTo(left + wordsX)
			r.cells(words[i].s, words[i].w)
		}
		out[top+y] = r.line()
	}
	x, y := 2+left, m.headRows()+top
	m.contactPic = rect{x, y + picTop, x + cols, y + picTop + rows}
	show := words[len(words)-1]
	x, y = x+wordsX, y+wordsTop+len(words)-1
	m.contactShow = rect{x, y, x + show.w, y + 1}
	return out
}

func widest(lines []line) int {
	w := 0
	for _, l := range lines {
		w = max(w, l.w)
	}
	return w
}

func (m *Model) contactWords(keep []int, show line, w int) []line {
	head, body, _ := m.readColours()
	words := []line{text(fg(head), clip(contactByline, w)), {}}
	for _, p := range keep {
		for _, s := range contactParagraphs[p] {
			words = append(words, text(fg(body), clip(s, w)))
		}
		words = append(words, line{})
	}
	return append(words, line{}, show)
}

func (m *Model) contactClipNow() *video.Video {
	if !m.contactClipRead {
		m.contactClipRead = true
		if m.opts.Media != "" {
			if b, err := os.ReadFile(filepath.Join(m.opts.Media, "noisebody.zkv")); err == nil {
				if m.contactClip, _ = video.Read(b); m.contactClip != nil {
					m.contactFrame = min(contactStill, m.contactClip.Frames()-1)
				}
			}
		}
	}
	return m.contactClip
}

const (
	contactStill = 6
	contactSpeed = 0.4
)

type contactTickMsg struct{}

func (m *Model) playContact() tea.Cmd {
	if m.contactPlaying {
		m.contactPlaying = false
		return nil
	}
	m.contactPlaying = true
	if m.contactTicking {
		return nil
	}
	m.contactTicking = true
	return m.contactTick()
}

func (m *Model) stopContact() {
	m.contactPlaying = false
	m.contactFrame = min(contactStill, m.contactClip.Frames()-1)
}

func (m *Model) contactTick() tea.Cmd {
	step := time.Duration(float64(time.Second) / (m.contactClip.FPS() * contactSpeed))
	return tea.Tick(step, func(time.Time) tea.Msg { return contactTickMsg{} })
}

func (m *Model) onContactTick() tea.Cmd {
	if m.section != secAbout || !m.contactPlaying {
		m.contactTicking = false
		if m.contactPlaying {
			m.stopContact()
		}
		return nil
	}
	if !m.contactOpen {
		m.contactFrame = (m.contactFrame + 1) % m.contactClip.Frames()
	}
	return m.contactTick()
}

func (m *Model) contactQRLines(w, h int) []line {
	codes := m.contactCodes()
	if len(codes) == 0 {
		return centre([]line{text(fg(m.theme.Warn), "the address does not fit a QR code")}, 0, w, h)
	}
	c := codes[len(codes)-1]
	for _, x := range codes {
		if x.code.Size <= w && (x.code.Size+1)/2 <= h {
			c = x
			break
		}
	}
	code, rows := c.code, (c.code.Size+1)/2
	dim := fg(m.theme.Dim)
	under := []line{{}}
	for _, s := range contactSteps {
		under = append(under, text(dim, clip(s, w)))
	}
	addr := contactAddress[:8] + "…" + contactAddress[len(contactAddress)-8:]
	if m.contactCopied {
		addr += " · copied"
	}
	under = append(under, line{}, text(dim, addr))
	if code.Size > w || rows > h {
		needW, needH := max(minW, code.Size+4), rows+m.headRows()+2
		if needH >= marketMinH {
			needH += 2
		}
		lines := []line{
			text(bold(m.theme.Text), "MAKE THE WINDOW BIGGER_"), {},
			text(dim, clip("the QR code needs "+itoa(needW)+" x "+itoa(needH)+", the window is "+itoa(m.w)+" x "+itoa(m.h), w)),
			{}, text(dim, addr),
		}
		for i := range lines {
			lines[i] = centreIn(lines[i], w)
		}
		return centre(lines, 0, w, h)
	}
	if rows+len(under) > h {
		under = under[:len(contactSteps)+1]
	}
	if rows+len(under) > h {
		under = nil
	}
	lines := append(m.qrLines(code, c.figure), under...)
	for i := range lines {
		lines[i] = centreIn(lines[i], w)
	}
	return centre(lines, 0, w, h)
}

type contactCode struct {
	code   *qr.Code
	figure bool
}

func (m *Model) contactCodes() []contactCode {
	if m.contactQR == nil {
		for _, c := range []struct {
			level  qr.Level
			figure bool
		}{{qr.H, true}, {qr.L, false}} {
			if code, err := qr.Encode(contactAddress, c.level); err == nil {
				m.contactQR = append(m.contactQR, contactCode{code, c.figure})
			}
		}
	}
	return m.contactQR
}

func (m *Model) qrLines(code *qr.Code, figure bool) []line {
	white, black := art.RGB{255, 255, 255}, m.theme.BodyBg
	fw := len(contactFigure)
	f0 := (code.Size - fw) / 2
	at := func(x, y int) art.RGB {
		if figure && x >= f0-1 && x < f0+fw+1 && y >= f0-1 && y < f0+fw+1 {
			if x >= f0 && y >= f0 && x < f0+fw && y < f0+fw && contactFigure[y-f0][x-f0] == '#' {
				return white
			}
			return black
		}
		if y < code.Size && code.Black(x, y) {
			return white
		}
		return black
	}
	var out []line
	for y := 0; y < code.Size; y += 2 {
		var r row
		for x := 0; x < code.Size; x++ {
			top, bot := at(x, y), at(x, y+1)
			switch {
			case top == bot:
				r.put(bg(top), " ")
			case m.opts.Glyph == art.LowerFiveEighths:
				r.put(fg(bot)+bg(top), "▅")
			default:
				r.put(fg(top)+bg(bot), "▀")
			}
		}
		out = append(out, r.line())
	}
	return out
}

func (m *Model) contactKey(k string) tea.Cmd {
	if k == "space" && m.contactClip != nil && !m.contactOpen {
		return m.playContact()
	}
	switch k {
	case "enter":
		m.contactOpen = true
	case "esc", "backspace":
		m.contactOpen, m.contactCopied = false, false
	case "c":
		if m.contactOpen {
			m.contactCopied = true
			return tea.SetClipboard(contactAddress)
		}
	}
	return nil
}

func (m *Model) contactClick(ms tea.Mouse) tea.Cmd {
	switch {
	case m.contactOpen:
	case m.contactShow.contains(ms.X, ms.Y):
		return m.contactKey("enter")
	case m.contactPic.contains(ms.X, ms.Y):
		return m.contactKey("space")
	}
	return nil
}

func (m *Model) contactHints() []line {
	if m.contactOpen {
		return []line{m.hints("esc", "back", "c", "copy address"), m.hints("esc", "back", "c", "copy")}
	}
	if m.contactPic == (rect{}) {
		return []line{m.hints("enter", "send message")}
	}
	play := "play"
	if m.contactPlaying {
		play = "stop"
	}
	return []line{m.hints("enter", "send message", "space", play), m.hints("enter", "send message")}
}
