package ui

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// Theme colours the chrome only; art keeps its own colours. Beside a few shared text
// colours every part of the frame has its own token, so a theme can change one part
// without touching the rest.
type Theme struct {
	Name string `json:"name"`

	Text   art.RGB `json:"text"`   // values and body text
	Dim    art.RGB `json:"dim"`    // secondary text, placeholders
	Accent art.RGB `json:"accent"` // highlights not covered by a token below
	Warn   art.RGB `json:"warn"`   // purged, errors
	Busy   art.RGB `json:"busy"`

	Border    art.RGB `json:"border"`
	Frame     art.RGB `json:"frame"`
	Corner    art.RGB `json:"corner"`
	CornerTip art.RGB `json:"cornerTip"`
	Scrollbar art.RGB `json:"scrollbar"` // scrollbar thumb
	BodyBg    art.RGB `json:"bodyBg"`    // inside the frame: black like the art's own background, so the art's edges show

	Brand    art.RGB `json:"brand"`    // zkSNARKs
	NavKey   art.RGB `json:"navKey"`   // the digits in [1]
	NavLabel art.RGB `json:"navLabel"` // inactive section names and brackets' text

	Prompt art.RGB `json:"prompt"` // the search line's [/]
	Input  art.RGB `json:"input"`  // typed query text

	Selection art.RGB `json:"selection"` // corner marks and the selected token's number
	Price     art.RGB `json:"price"`     // asking prices under thumbnails

	ListSel  art.RGB `json:"listSel"`  // the selected row in the list
	ListHead art.RGB `json:"listHead"` // the list's column names, as quiet as the market strip's words

	Market      art.RGB `json:"market"`      // the market strip's values
	MarketLabel art.RGB `json:"marketLabel"` // the words around them
	MarketSep   art.RGB `json:"marketSep"`   // the "~" between the strip's parts
	Status      art.RGB `json:"status"`      // network status
	HintKey     art.RGB `json:"hintKey"`     // keys in [ENTER]
	HintBracket art.RGB `json:"hintBracket"` // brackets and separators in key hints
	HintLabel   art.RGB `json:"hintLabel"`   // what a key does

	Heading     art.RGB `json:"heading"`     // card headings and field names
	TraitBorder art.RGB `json:"traitBorder"` // the frame around the card's traits
	TraitFloor  art.RGB `json:"traitFloor"`  // the traits' floor prices and their "Floor in ZEC" label

	ModalBg      art.RGB `json:"modalBg"`      // the card's background
	ModalBorder  art.RGB `json:"modalBorder"`  // the card's frame
	ModalTitleBg art.RGB `json:"modalTitleBg"` // the token number on the modal's frame
	ModalTitleFg art.RGB `json:"modalTitleFg"`
	Shadow       float64 `json:"shadow"`   // how much the modal's shadow darkens what is under it
	Backdrop     float64 `json:"backdrop"` // how much the screen behind an open card is darkened
}

const (
	frameRung                     = 0.85
	tipRung, cornerRung, lineRung = 3, 7, 10
)

func frameTone(lines art.RGB, k int) art.RGB {
	v := 255 * math.Pow(frameRung, float64(k))
	lum := 0.2126*float64(lines[0]) + 0.7152*float64(lines[1]) + 0.0722*float64(lines[2])
	var tint, grey art.RGB
	for i := range tint {
		tint[i] = uint8(min(255, v*float64(lines[i])/lum+0.5))
		grey[i] = uint8(v + 0.5)
	}
	return mix(tint, grey, v/255)
}

// palette builds a theme from a handful of colours, deriving every token.
func palette(name string, border, accent, text, dim, warn, busy, panel art.RGB) Theme {
	return Theme{
		Name: name, Text: text, Dim: dim, Accent: accent, Warn: warn, Busy: busy,
		Border: border, Frame: frameTone(border, lineRung), Corner: frameTone(border, cornerRung), CornerTip: frameTone(border, tipRung), Scrollbar: accent,
		Brand: accent, NavKey: accent, NavLabel: dim,
		Prompt: accent, Input: text,
		Selection: accent, Price: text,
		BodyBg: art.RGB{}, ListSel: mix(art.RGB{}, border, 0.3), ListHead: mix(dim, art.RGB{}, 0.3),
		Market: dim, MarketLabel: mix(dim, art.RGB{}, 0.3), MarketSep: mix(dim, art.RGB{}, 0.45), Status: dim, HintKey: accent, HintBracket: border, HintLabel: dim,
		Heading: dim, TraitBorder: mix(border, panel, 0.3), TraitFloor: mix(dim, panel, 0.3),
		ModalBg: panel, ModalBorder: border, ModalTitleBg: border, ModalTitleFg: art.RGB{255, 255, 255},
		Shadow: 0.35, Backdrop: 0.5,
	}
}

var themes = []Theme{
	// ink with green instead of yellow, on a darker modal
	palette("green", art.RGB{90, 90, 102}, art.RGB{120, 255, 150}, art.RGB{228, 228, 231}, art.RGB{139, 139, 148}, art.RGB{239, 107, 107}, art.RGB{244, 211, 94}, art.RGB{17, 17, 20}),
	palette("ink", art.RGB{90, 90, 102}, art.RGB{244, 211, 94}, art.RGB{228, 228, 231}, art.RGB{139, 139, 148}, art.RGB{239, 107, 107}, art.RGB{244, 211, 94}, art.RGB{26, 26, 30}),
	palette("red", art.RGB{90, 90, 102}, art.RGB{255, 83, 83}, art.RGB{228, 228, 231}, art.RGB{139, 139, 148}, art.RGB{239, 107, 107}, art.RGB{244, 211, 94}, art.RGB{17, 17, 20}),
	palette("zec", art.RGB{120, 98, 52}, art.RGB{244, 183, 40}, art.RGB{240, 232, 214}, art.RGB{160, 146, 118}, art.RGB{239, 107, 107}, art.RGB{244, 183, 40}, art.RGB{30, 26, 20}),
	palette("phosphor", art.RGB{46, 110, 66}, art.RGB{120, 255, 150}, art.RGB{200, 240, 205}, art.RGB{98, 160, 112}, art.RGB{255, 120, 100}, art.RGB{244, 211, 94}, art.RGB{16, 28, 20}),
	palette("ice", art.RGB{70, 96, 130}, art.RGB{110, 200, 255}, art.RGB{220, 232, 245}, art.RGB{128, 148, 172}, art.RGB{255, 120, 130}, art.RGB{244, 211, 94}, art.RGB{20, 26, 34}),
}

const reset = "\x1b[0m"

func fg(c art.RGB) string {
	return "\x1b[38;2;" + strconv.Itoa(int(c[0])) + ";" + strconv.Itoa(int(c[1])) + ";" + strconv.Itoa(int(c[2])) + "m"
}

func bg(c art.RGB) string { return strings.Replace(fg(c), "38;", "48;", 1) }

func bold(c art.RGB) string { return "\x1b[1m" + fg(c) }

// onBg puts styled s on background c: wherever s goes back to the terminal's own
// background, it gets c instead.
func onBg(s string, c art.RGB) string {
	b := bg(c)
	s = strings.ReplaceAll(s, reset, reset+b)
	s = strings.ReplaceAll(s, "\x1b[49m", b)
	return b + s + reset
}

// mix moves c towards to by k: 0 keeps c, 1 gives to.
func mix(c, to art.RGB, k float64) art.RGB {
	var out art.RGB
	for i := range out {
		out[i] = uint8(float64(c[i]) + (float64(to[i])-float64(c[i]))*k + 0.5)
	}
	return out
}

// width is the display width of chrome text: everything we draw is single-width.
func width(s string) int { return utf8.RuneCountInString(s) }

// clip cuts s to at most n cells, marking the cut with an ellipsis.
func clip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if width(s) <= n {
		return s
	}
	r := []rune(s)
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// row builds one line of styled cells and tracks its width.
type row struct {
	b strings.Builder
	w int
}

func (r *row) put(style, s string) {
	if s == "" {
		return
	}
	if style != "" {
		r.b.WriteString(style)
	}
	r.b.WriteString(s)
	if style != "" {
		r.b.WriteString(reset)
	}
	r.w += width(s)
}

// cells appends pre-styled content of a known width, such as a line of art.
func (r *row) cells(s string, w int) {
	r.b.WriteString(s)
	r.w += w
}

func (r *row) space(n int) {
	if n > 0 {
		r.b.WriteString(strings.Repeat(" ", n))
		r.w += n
	}
}

func (r *row) padTo(n int) { r.space(n - r.w) }

func (r *row) String() string { return r.b.String() }

// line is a finished line of content with its width.
type line struct {
	s string
	w int
}

func (r *row) line() line { return line{r.String(), r.w} }

func text(style, s string) line {
	var r row
	r.put(style, s)
	return r.line()
}

// leftRight places l at the left and r at the right of n cells, clipping l if needed.
func leftRight(lStyle, l, rStyle, rt string, n int) line {
	var b row
	if width(l)+1+width(rt) > n {
		l = clip(l, n-1-width(rt))
	}
	b.put(lStyle, l)
	b.padTo(n - width(rt))
	b.put(rStyle, rt)
	return b.line()
}

// hint renders "[KEY] LABEL" pairs: only the key takes the accent; brackets and separators
// use the border colour and the label the dim text colour.
func (m *Model) hints(pairs ...string) line {
	var r row
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			r.space(2)
		}
		m.keyHint(&r, pairs[i])
		r.space(1)
		r.put(fg(m.theme.HintLabel), strings.ToUpper(pairs[i+1]))
	}
	return r.line()
}

// keyHint draws "[KEY]"; arrow pairs get a slash between them, and separators inside a
// key (←/→, 1-4) take the bracket colour.
func (m *Model) keyHint(r *row, key string) { m.keyIn(r, key, m.theme.HintKey, m.theme.HintBracket) }

// keyIn draws "[KEY]" in the given key and bracket colours.
func (m *Model) keyIn(r *row, key string, keyC, bracketC art.RGB) {
	key = strings.NewReplacer("←→", "←/→", "↑↓", "↑/↓").Replace(strings.ToUpper(key))
	r.put(fg(bracketC), "[")
	runes := []rune(key)
	for i, c := range runes {
		sep := (c == '/' || c == '-') && i > 0 && i < len(runes)-1 // "/" alone is a key
		if sep {
			r.put(fg(bracketC), string(c))
		} else {
			r.put(fg(keyC), string(c))
		}
	}
	r.put(fg(bracketC), "]")
}

// truncations counts the lines truncate had to cut, so tests can check that none were.
var truncations int

// truncate cuts a styled line to n cells, keeping its escape sequences intact. It is a
// safety net: layouts are meant to fit, and a cut line is a layout bug.
func truncate(l line, n int) line {
	if l.w <= n {
		return l
	}
	truncations++
	var b strings.Builder
	w := 0
	for i := 0; i < len(l.s); {
		if l.s[i] == 0x1b && i+1 < len(l.s) && l.s[i+1] == ']' { // OSC, such as a hyperlink
			j := oscEnd(l.s, i)
			b.WriteString(l.s[i:j])
			i = j
			continue
		}
		if l.s[i] == 0x1b {
			j := i + 1
			for j < len(l.s) && !(l.s[j] >= 0x40 && l.s[j] <= 0x7e && j > i+1) {
				j++
			}
			b.WriteString(l.s[i : j+1])
			i = j + 1
			continue
		}
		if w == n {
			break
		}
		_, size := utf8.DecodeRuneInString(l.s[i:])
		b.WriteString(l.s[i : i+size])
		i += size
		w++
	}
	b.WriteString(reset)
	return line{b.String(), w}
}

// oscEnd returns the index just past the OSC sequence starting at s[i], ended by ST
// (ESC \\) or BEL.
func oscEnd(s string, i int) int {
	for j := i + 2; j < len(s); j++ {
		if s[j] == 0x07 {
			return j + 1
		}
		if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
			return j + 2
		}
	}
	return len(s)
}
