package ui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// cell is one terminal cell.
type cell struct {
	r            rune
	fg, bg       art.RGB
	hasFg, hasBg bool
	bold, italic bool
	link         string // OSC 8 hyperlink target
}

// canvas is a grid of cells for screens that layer one thing over another, such as a modal
// with a shadow over the gallery. Content is drawn from the same styled strings the rest of
// the UI builds; only the SGR subset this package emits is understood.
type canvas struct {
	w, h int
	c    []cell
	flat bool // shade does nothing, for a caller that draws its own shadows
}

func newCanvas(w, h int) *canvas {
	cv := &canvas{w: w, h: h, c: make([]cell, w*h)}
	for i := range cv.c {
		cv.c[i].r = ' '
	}
	return cv
}

func (cv *canvas) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= cv.w || y >= cv.h {
		return nil
	}
	return &cv.c[y*cv.w+x]
}

// draw writes a styled string at x, y. Cells the string gives no background keep theirs, so
// text can sit on a filled box.
func (cv *canvas) draw(x, y int, s string) {
	var st cell
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == ']' {
			j := oscEnd(s, i)
			if body := strings.TrimRight(s[i+2:j], "\x1b\\\a"); strings.HasPrefix(body, "8;") {
				_, url, _ := strings.Cut(body[2:], ";")
				st.link = url
			}
			i = j
			continue
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			applySGR(&st, s[i+2:j])
			i = j + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if c := cv.at(x, y); c != nil {
			bg, hasBg := c.bg, c.hasBg
			*c = st
			c.r = r
			if !st.hasBg {
				c.bg, c.hasBg = bg, hasBg
			}
		}
		x++
	}
}

func applySGR(st *cell, params string) {
	p := strings.Split(params, ";")
	for k := 0; k < len(p); k++ {
		switch p[k] {
		case "", "0":
			*st = cell{link: st.link} // a reset ends the style, not the link
		case "1":
			st.bold = true
		case "3":
			st.italic = true
		case "38", "48":
			if k+4 < len(p) && p[k+1] == "2" {
				var c art.RGB
				for n := 0; n < 3; n++ {
					v, _ := strconv.Atoi(p[k+2+n])
					c[n] = uint8(v)
				}
				if p[k] == "38" {
					st.fg, st.hasFg = c, true
				} else {
					st.bg, st.hasBg = c, true
				}
				k += 4
			}
		case "39":
			st.hasFg = false
		case "49":
			st.hasBg = false
		}
	}
}

// fill paints a rectangle's background and clears its text.
func (cv *canvas) fill(x0, y0, x1, y1 int, bg art.RGB) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if c := cv.at(x, y); c != nil {
				*c = cell{r: ' ', bg: bg, hasBg: true}
			}
		}
	}
}

func scale(c art.RGB, k float64) art.RGB {
	return art.RGB{uint8(float64(c[0]) * k), uint8(float64(c[1]) * k), uint8(float64(c[2]) * k)}
}

// shade darkens a rectangle the way a shadow does: what is underneath stays legible, dimly.
// Cells on the terminal's own background get black, since its colour is unknown.
func (cv *canvas) shade(x0, y0, x1, y1 int, k float64, fallback art.RGB) {
	if cv.flat {
		return
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := cv.at(x, y)
			if c == nil {
				continue
			}
			if c.hasBg {
				c.bg = scale(c.bg, k)
			} else {
				c.bg, c.hasBg = fallback, true
			}
			if c.hasFg {
				c.fg = scale(c.fg, k)
			} else {
				c.fg, c.hasFg = scale(art.RGB{200, 200, 200}, k), true
			}
			c.bold = false
		}
	}
}

// String serialises the canvas, one line per row.
func (cv *canvas) String() string {
	var b strings.Builder
	for y := 0; y < cv.h; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		var cur cell
		started := false
		link := ""
		for x := 0; x < cv.w; x++ {
			c := cv.c[y*cv.w+x]
			if c.link != link {
				b.WriteString("\x1b]8;;" + c.link + "\x1b\\")
				link = c.link
			}
			if !started || c.fg != cur.fg || c.bg != cur.bg || c.hasFg != cur.hasFg || c.hasBg != cur.hasBg || c.bold != cur.bold || c.italic != cur.italic {
				b.WriteString(reset)
				if c.bold {
					b.WriteString("\x1b[1m")
				}
				if c.italic {
					b.WriteString("\x1b[3m")
				}
				if c.hasFg {
					b.WriteString(fg(c.fg))
				}
				if c.hasBg {
					b.WriteString(strings.Replace(fg(c.bg), "38;", "48;", 1))
				}
				cur, started = c, true
			}
			b.WriteRune(c.r)
		}
		if link != "" {
			b.WriteString("\x1b]8;;\x1b\\")
		}
		b.WriteString(reset)
	}
	return b.String()
}
