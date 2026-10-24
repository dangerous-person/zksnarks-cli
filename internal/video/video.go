// Package video reads the .zkv clips the network section plays: terminal frames in
// half-blocks (two square pixels per cell) or symbols (one character per cell in that cell's
// colour), scene after scene.
//
// A file is a gzip stream of scenes, each a ZKV1 (half-blocks) or ZKC1 (symbols) stream
// with its own header, palette and keyframes; all scenes have the same size in cells and
// the same frame rate.
package video

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

const same = 255 // "same as the previous frame"

// scene is one stream of the file.
type scene struct {
	symbols  []string // nil for half-blocks
	fill     int
	w, h     int // half-blocks: pixels, the picture is h/2 cells high; symbols: cells
	fps      float64
	keyEvery int
	palette  []art.RGB
	frames   [][]byte // palette indices or same; for symbols (symbol, colour) pairs
	first    int      // its first frame in the whole clip
	plane    []byte   // the frame decoded last
}

// cells is the picture's size in terminal cells.
func (s *scene) cells() (cols, rows int) {
	if s.symbols != nil {
		return s.w, s.h
	}
	return s.w, s.h / 2
}

// Video is a decoded clip. It keeps the frame it showed last, so playing on is cheap; it is
// not safe for concurrent use.
type Video struct {
	scenes []*scene
	count  int
	shown  int // the frame the planes hold, -1 for none
	lines  []string
	glyph  art.Glyph
}

// Read decodes a .zkv file.
func Read(data []byte) (*Video, error) {
	z, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(z)
	if err != nil {
		return nil, err
	}
	v := &Video{shown: -1}
	for len(raw) > 0 {
		s, rest, err := readScene(raw)
		if err != nil {
			return nil, err
		}
		if len(v.scenes) > 0 {
			c0, r0 := v.scenes[0].cells()
			if c, r := s.cells(); c != c0 || r != r0 || s.fps != v.scenes[0].fps {
				return nil, errors.New("video: the scenes differ in size or frame rate")
			}
		}
		s.first = v.count
		s.plane = make([]byte, len(s.frames[0]))
		v.count += len(s.frames)
		v.scenes = append(v.scenes, s)
		raw = rest
	}
	if v.count == 0 {
		return nil, errors.New("video: no frames")
	}
	return v, nil
}

func readScene(data []byte) (*scene, []byte, error) {
	magic := string(data[:min(4, len(data))])
	if len(data) < 18 || (magic != "ZKV1" && magic != "ZKC1" && magic != "ZKC2") {
		return nil, nil, errors.New("video: not a ZKV1, ZKC1 or ZKC2 stream")
	}
	le := binary.LittleEndian
	s := &scene{
		w:        int(le.Uint16(data[4:])),
		h:        int(le.Uint16(data[6:])),
		fps:      float64(le.Uint16(data[8:])) / 100,
		keyEvery: int(le.Uint16(data[14:])),
	}
	count, np := int(le.Uint32(data[10:])), int(le.Uint16(data[16:]))
	head, n := 18+np*3, s.w*s.h
	if len(data) < head {
		return nil, nil, errors.New("video: damaged file")
	}
	if magic != "ZKV1" {
		if len(data) <= head {
			return nil, nil, errors.New("video: damaged file")
		}
		l := int(data[head])
		for _, r := range string(data[head+1 : min(len(data), head+1+l)]) {
			s.symbols = append(s.symbols, string(r))
		}
		head, n = head+1+l, n*2
		if magic == "ZKC2" {
			if len(data) <= head {
				return nil, nil, errors.New("video: damaged file")
			}
			s.fill, head = int(data[head]), head+1
		}
	}
	if s.w == 0 || (s.symbols == nil && s.h%2 != 0) || s.fps <= 0 || s.keyEvery == 0 || count == 0 || len(data) < head+count*n {
		return nil, nil, errors.New("video: damaged file")
	}
	for i := range np {
		o := 18 + i*3
		s.palette = append(s.palette, art.RGB{data[o], data[o+1], data[o+2]})
	}
	for i := range count {
		s.frames = append(s.frames, data[head+i*n:head+(i+1)*n])
	}
	return s, data[head+count*n:], nil
}

// Size is the picture's size in terminal cells.
func (v *Video) Size() (cols, rows int) { return v.scenes[0].cells() }

// Frames is the number of frames.
func (v *Video) Frames() int { return v.count }

// FPS is the frame rate.
func (v *Video) FPS() float64 { return v.scenes[0].fps }

// Duration is how long the clip plays.
func (v *Video) Duration() time.Duration {
	return time.Duration(float64(v.count) / v.FPS() * float64(time.Second))
}

// At is the frame on screen at time d.
func (v *Video) At(d time.Duration) int {
	return max(0, min(v.count-1, int(d.Seconds()*v.FPS())))
}

// Frame renders frame i as one string of ANSI cells per row; half-blocks are split with g.
func (v *Video) Frame(i int, g art.Glyph) []string {
	i = max(0, min(v.count-1, i))
	if i == v.shown && g == v.glyph && v.lines != nil {
		return v.lines
	}
	s := v.at(i)
	if v.shown >= 0 && v.at(v.shown) != s {
		v.shown = -1
	}
	s.show(v.shown-s.first, i-s.first)
	v.shown, v.glyph = i, g
	if s.symbols != nil {
		v.lines = s.drawSymbols()
	} else {
		v.lines = s.drawBlocks(g)
	}
	return v.lines
}

// at is the scene that holds frame i of the clip.
func (v *Video) at(i int) *scene {
	for k := len(v.scenes) - 1; k > 0; k-- {
		if i >= v.scenes[k].first {
			return v.scenes[k]
		}
	}
	return v.scenes[0]
}

// show brings the plane to the scene's frame i, given that it holds frame shown (negative
// for nothing): frames run on from there, or from the keyframe before i when that is closer.
func (s *scene) show(shown, i int) {
	key := i - i%s.keyEvery
	from := shown + 1
	if shown < 0 || i < shown || key > from {
		from = key
	}
	for k := from; k <= i; k++ {
		f := s.frames[k]
		if s.symbols != nil {
			for j := 0; j < len(f); j += 2 {
				if f[j] != same {
					s.plane[j], s.plane[j+1] = f[j], f[j+1]
				}
			}
			continue
		}
		for j, c := range f {
			if c != same {
				s.plane[j] = c
			}
		}
	}
}

func (s *scene) colour(c byte) art.RGB {
	if int(c) < len(s.palette) {
		return s.palette[c]
	}
	return art.RGB{}
}

// drawBlocks draws two pixels per cell, like the art's thumbnails.
func (s *scene) drawBlocks(g art.Glyph) []string {
	lines := make([]string, s.h/2)
	for y := range lines {
		var w sgr
		for x := 0; x < s.w; x++ {
			top, bot := s.colour(s.plane[2*y*s.w+x]), s.colour(s.plane[(2*y+1)*s.w+x])
			switch {
			case top == bot:
				w.cell(paint{}, paint{top, true}, " ")
			case g == art.LowerFiveEighths:
				w.cell(paint{bot, true}, paint{top, true}, "▅")
			default:
				w.cell(paint{top, true}, paint{bot, true}, "▀")
			}
		}
		lines[y] = w.end()
	}
	return lines
}

func (s *scene) drawSymbols() []string {
	lines := make([]string, s.h)
	for y := range lines {
		var w sgr
		for x := 0; x < s.w; x++ {
			k, c := int(s.plane[(y*s.w+x)*2]), s.plane[(y*s.w+x)*2+1]
			sym := " "
			if k < len(s.symbols) {
				sym = s.symbols[k]
			}
			fg, bg := paint{s.colour(c), true}, paint{}
			if s.fill > 0 && sym != " " {
				bg = paint{dim(fg.rgb, s.fill), true}
			}
			w.cell(fg, bg, sym)
		}
		lines[y] = w.end()
	}
	return lines
}

func dim(c art.RGB, f int) art.RGB {
	for i := range c {
		c[i] = byte((int(c[i])*f + 127) / 255)
	}
	return c
}

// paint is a colour, or the terminal's own when unset.
type paint struct {
	rgb art.RGB
	set bool
}

// sgr writes truecolor cells, emitting a colour only when it changes.
type sgr struct {
	b      strings.Builder
	fg, bg paint
	begun  bool
}

// cell writes s on bg in fg; a space needs no fg.
func (w *sgr) cell(fg, bg paint, s string) {
	if !w.begun || w.bg != bg {
		w.colour(48, bg)
		w.bg = bg
	}
	if s != " " && (!w.begun || w.fg != fg) {
		w.colour(38, fg)
		w.fg = fg
	}
	w.begun = true
	w.b.WriteString(s)
}

func (w *sgr) colour(kind int, p paint) {
	if !p.set {
		w.b.WriteString("\x1b[" + strconv.Itoa(kind+1) + "m") // 39 or 49: the terminal's own
		return
	}
	w.b.WriteString("\x1b[")
	w.b.WriteString(strconv.Itoa(kind))
	w.b.WriteString(";2;")
	w.b.WriteString(strconv.Itoa(int(p.rgb[0])))
	w.b.WriteByte(';')
	w.b.WriteString(strconv.Itoa(int(p.rgb[1])))
	w.b.WriteByte(';')
	w.b.WriteString(strconv.Itoa(int(p.rgb[2])))
	w.b.WriteByte('m')
}

func (w *sgr) end() string {
	w.b.WriteString("\x1b[0m")
	return w.b.String()
}
