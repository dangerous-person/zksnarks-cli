package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

func (m *Model) manifestoKey(k string) tea.Cmd {
	if !m.reading {
		if k == "enter" || k == "space" {
			m.reading, m.readTop = true, 0
		}
		return nil
	}
	page := max(1, m.bodyH()-2)
	switch k {
	case "esc", "backspace", "enter":
		m.reading = false
	case "up":
		m.readTop--
	case "down":
		m.readTop++
	case "pgup", "ctrl+b", "ctrl+u":
		m.readTop -= page
	case "pgdown", "ctrl+f", "ctrl+d", "space":
		m.readTop += page
	case "home", "g":
		m.readTop = 0
	case "end", "G":
		m.readTop = 1 << 30
	}
	return nil
}

// word is one run of non-space text, possibly mixing highlighted and plain parts.
type word struct {
	parts []site.Span
	w     int
}

func words(spans []site.Span) []word {
	var out []word
	var cur word
	flush := func() {
		if cur.w > 0 {
			out = append(out, cur)
		}
		cur = word{}
	}
	for _, sp := range spans {
		for i, f := range strings.Split(sp.Text, " ") {
			if i > 0 {
				flush()
			}
			if f != "" {
				cur.parts = append(cur.parts, site.Span{Text: f, Hi: sp.Hi})
				cur.w += width(f)
			}
		}
	}
	flush()
	return out
}

// wrap breaks spans into lines of at most w cells.
func wrap(spans []site.Span, w int) [][]site.Span {
	var lines [][]site.Span
	var cur []site.Span
	cw := 0
	for _, wd := range words(spans) {
		if cw > 0 && cw+1+wd.w > w {
			lines = append(lines, cur)
			cur, cw = nil, 0
		}
		if cw > 0 {
			cur = append(cur, site.Span{Text: " ", Hi: cur[len(cur)-1].Hi && wd.parts[0].Hi})
			cw++
		}
		cur = append(cur, wd.parts...)
		cw += wd.w
	}
	if cw > 0 {
		lines = append(lines, cur)
	}
	return lines
}

func plain(s string) []site.Span { return []site.Span{{Text: s}} }

// spanLine draws at most limit characters of a wrapped line and reports how many it drew.
func spanLine(r *row, spans []site.Span, normal, hi string, limit int) int {
	n := 0
	for _, s := range spans {
		if n >= limit {
			break
		}
		t := s.Text
		if rest := limit - n; width(t) > rest {
			t = string([]rune(t)[:rest])
		}
		style := normal
		if s.Hi {
			style = hi
		}
		r.put(style, t)
		n += width(t)
	}
	return n
}

func spansWidth(spans []site.Span) int {
	n := 0
	for _, s := range spans {
		n += width(s.Text)
	}
	return n
}

func centre(lines []line, left, w, h int) []line {
	out := make([]line, h)
	top := max(0, (h-len(lines))/2)
	for i, l := range lines {
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

func (m *Model) manifestoLines(w, h int) ([]line, func(int) bool) {
	if m.reading {
		return m.archiveLines(w, h)
	}
	mf := m.opts.Site.Manifesto
	cw := min(64, w-4)
	left := (w - cw) / 2
	_, body, _ := m.readColours()
	normal := fg(body)
	var block []line
	paragraph := func(spans []site.Span) {
		for _, ln := range wrap(spans, cw) {
			var r row
			spanLine(&r, ln, normal, normal, 1<<30)
			block = append(block, r.line())
		}
	}
	paragraph(mf.Lead)
	block = append(block, line{})
	paragraph(plain(mf.Body))
	block = append(block, line{}, line{}, m.pageKey("enter", "read the cypherpunk's manifesto"))
	return centre(block, left, w, h), nil
}

var manifestoHeading = []string{
	`                              .__                                      __     /\`,
	`_____      ____ ___.__.______ |  |__   _________________  __ __  ____ |  | __ )/______`,
	`\__  \   _/ ___<   |  |\____ \|  |  \_/ __ \_  __ \____ \|  |  \/    \|  |/ /  /  ___/`,
	` / __ \_ \  \___\___  ||  |_> >   Y  \  ___/|  | \/  |_> >  |  /   |  \    <   \___ \`,
	`(____  /  \___  > ____||   __/|___|  /\___  >__|  |   __/|____/|___|  /__|_ \ /____  >`,
	`     \/       \/\/     |__|        \/     \/      |__|              \/     \/      \/`,
	`                      .__  _____                __`,
	`  _____ _____    ____ |__|/ ____\____   _______/  |_  ____`,
	` /     \\__  \  /    \|  \   __\/ __ \ /  ___/\   __\/  _ \`,
	`|  Y Y  \/ __ \|   |  \  ||  | \  ___/ \___ \  |  | (  <_> )`,
	`|__|_|  (____  /___|  /__||__|  \___  >____  > |__|  \____/`,
	`      \/     \/     \/              \/     \/`,
}

func (m *Model) readColours() (head, body, mark art.RGB) {
	white := art.RGB{255, 255, 255}
	return mix(m.theme.BodyBg, white, 0.45), mix(m.theme.Dim, white, 0.2), mix(m.theme.Dim, white, 0.76)
}

func (m *Model) archiveLines(w, h int) ([]line, func(int) bool) {
	mf := m.opts.Site.Manifesto
	cw := min(72, w)
	headW := 0
	for _, l := range manifestoHeading {
		headW = max(headW, width(l))
	}
	head, body, mark := m.readColours()
	doc := []line{{}}
	if headW <= w {
		for _, l := range manifestoHeading {
			doc = append(doc, text(fg(head), l))
		}
		doc = append(doc, line{})
	} else {
		doc = append(doc, text(bold(head), clip(mf.Title, cw)))
	}
	name, email, _ := strings.Cut(mf.Signature, " <")
	email = strings.TrimSuffix(email, ">")
	if name != "" {
		doc = append(doc, text(fg(head), clip("by "+name, cw)), line{})
	}
	for i, p := range mf.Paras {
		normal := fg(body)
		if i == len(mf.Paras)-1 && spansWidth(p) < 12 {
			normal = bold(mark)
		}
		for _, ln := range wrap(p, cw) {
			var r row
			spanLine(&r, ln, normal, bold(mark), 1<<30)
			doc = append(doc, r.line())
		}
		doc = append(doc, line{})
	}
	if name != "" {
		doc = append(doc, text(fg(body), clip(name, cw)))
		if email != "" {
			doc = append(doc, text(fg(body), clip(email, cw)))
		}
		doc = append(doc, line{}, text(fg(m.theme.Dim), clip(mf.Date, cw)), line{})
	}
	m.readTop = max(0, min(m.readTop, len(doc)-h))
	out := make([]line, h)
	copy(out, doc[m.readTop:])
	if len(doc) <= h {
		return out, nil
	}
	size := max(1, h*h/len(doc))
	pos := m.readTop * (h - size) / max(1, len(doc)-h)
	return out, func(i int) bool { return i >= pos && i < pos+size }
}

func centreIn(l line, w int) line {
	var r row
	r.space(max(0, (w-l.w)/2))
	r.cells(l.s, l.w)
	return r.line()
}

func (m *Model) spotlight() (site.Spotlight, bool) {
	s := m.opts.Site.Spotlights
	if len(s) == 0 {
		return site.Spotlight{}, false
	}
	return s[m.spot], true
}
