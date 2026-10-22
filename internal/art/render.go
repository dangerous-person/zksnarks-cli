package art

import (
	"strconv"
	"strings"
)

// Glyph selects how a thumbnail cell is split between two art pixels stacked vertically.
type Glyph int

const (
	// UpperHalf draws ▀ with fg = top pixel, bg = bottom pixel. It is exact in terminals that
	// draw block elements to the cell edges themselves (Ghostty, kitty, WezTerm, VS Code).
	UpperHalf Glyph = iota
	// LowerFiveEighths draws ▅ with fg = bottom pixel, bg = top pixel. Terminal.app takes block
	// glyphs from the font: with SF Mono the glyph box is the bottom 27 of 34 px, so ▀ leaves a
	// 7 px strip of the wrong colour while ▅ splits the cell exactly 17/17.
	LowerFiveEighths
)

const (
	ThumbW, ThumbH = Grid, Grid / 2 // 26x13 cells
	CardW, CardH   = Grid * 2, Grid // 52x26 cells
)

// brailleBit[row][col] is the bit of a braille dot slot (2 columns x 4 rows per cell).
var brailleBit = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// lattice holds the braille glyph for each combination of DenseX and DenseY: a sparse axis
// uses every other dot slot, a dense one every slot.
var lattice = func() [4]rune {
	var out [4]rune
	for f := 0; f < 4; f++ {
		denseX, denseY := f&1 != 0, f&2 != 0
		mask := rune(0)
		for j := 0; j < 4; j++ {
			for i := 0; i < 2; i++ {
				if (denseX || i%2 == 0) && (denseY || j%2 == 0) {
					mask |= brailleBit[j][i]
				}
			}
		}
		out[f] = 0x2800 + mask
	}
	return out
}()

// Grey returns the art in greyscale, the way the collection shows purged tokens. The black
// background stays black.
func Grey(a Art) Art {
	g := a
	g.Palette = make([]RGB, len(a.Palette))
	for i, c := range a.Palette {
		if c == (RGB{}) {
			continue
		}
		y := (299*int(c[0]) + 587*int(c[1]) + 114*int(c[2])) / 1000
		y = 48 + y*3/4 // lift the darks a little so the shape survives on black
		g.Palette[i] = RGB{uint8(y), uint8(y), uint8(y)}
	}
	return g
}

// Thumb renders the art as 26x13 cells, two art pixels per cell. Pixel-exact.
func Thumb(a Art, g Glyph) []string {
	lines := make([]string, 0, ThumbH)
	for y := 0; y < Grid; y += 2 {
		var w sgrWriter
		for x := 0; x < Grid; x++ {
			top, bot := a.Base(x, y), a.Base(x, y+1)
			switch {
			case top == bot:
				w.cell(RGB{}, top, ' ', false)
			case g == LowerFiveEighths:
				w.cell(bot, top, '▅', true)
			default:
				w.cell(top, bot, '▀', true)
			}
		}
		lines = append(lines, w.end())
	}
	return lines
}

// Card renders the art as 52x26 cells: every art pixel is two cells side by side, so pixel
// edges always fall on cell edges. Texture is redrawn as an even braille lattice instead of
// being sampled; sampling an 8 px lattice onto braille slots aliases into dirty patterns.
func Card(a Art) []string {
	lines := make([]string, 0, CardH)
	for y := 0; y < Grid; y++ {
		var w sgrWriter
		for x := 0; x < Grid; x++ {
			bg := a.Base(x, y)
			flags, dot := a.Texel(x, y)
			for range 2 {
				if flags&Dotted == 0 {
					w.cell(RGB{}, bg, ' ', false)
					continue
				}
				f := 0
				if flags&DenseX != 0 {
					f |= 1
				}
				if flags&DenseY != 0 {
					f |= 2
				}
				w.cell(dot, bg, lattice[f], true)
			}
		}
		lines = append(lines, w.end())
	}
	return lines
}

// sgrWriter writes truecolor cells, emitting a colour only when it changes.
type sgrWriter struct {
	b            strings.Builder
	fg, bg       RGB
	hasFg, hasBg bool
}

func (w *sgrWriter) cell(fg, bg RGB, r rune, useFg bool) {
	if !w.hasBg || w.bg != bg {
		w.colour(48, bg)
		w.bg, w.hasBg = bg, true
	}
	if useFg && (!w.hasFg || w.fg != fg) {
		w.colour(38, fg)
		w.fg, w.hasFg = fg, true
	}
	w.b.WriteRune(r)
}

func (w *sgrWriter) colour(kind int, c RGB) {
	w.b.WriteString("\x1b[")
	w.b.WriteString(strconv.Itoa(kind))
	w.b.WriteString(";2;")
	w.b.WriteString(strconv.Itoa(int(c[0])))
	w.b.WriteByte(';')
	w.b.WriteString(strconv.Itoa(int(c[1])))
	w.b.WriteByte(';')
	w.b.WriteString(strconv.Itoa(int(c[2])))
	w.b.WriteByte('m')
}

func (w *sgrWriter) end() string {
	w.b.WriteString("\x1b[0m")
	return w.b.String()
}
