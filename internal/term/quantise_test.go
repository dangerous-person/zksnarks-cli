package term

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

func TestQuantiserDarkGreys(t *testing.T) {
	q := NewQuantiser(colorprofile.ANSI256)
	for _, c := range []struct {
		in   art.RGB
		want uint8
	}{
		{art.RGB{0x2c, 0x31, 0x38}, 236},
		{art.RGB{0x26, 0x2a, 0x30}, 235},
		{art.RGB{0x3c, 0x4b, 0x1e}, 238},
	} {
		if got := q.Index(c.in); got != c.want {
			t.Errorf("%v: got %d, want %d", c.in, got, c.want)
		}
	}
}

func TestQuantiserPaletteMapsToItself(t *testing.T) {
	q := NewQuantiser(colorprofile.ANSI256)
	named := map[art.RGB]uint8{
		{0, 0, 0}: 16, {255, 255, 255}: 231,
		{255, 0, 0}: 196, {0, 255, 0}: 46, {0, 0, 255}: 21,
		{0, 255, 255}: 51, {255, 0, 255}: 201, {255, 255, 0}: 226,
	}
	for c, want := range named {
		if got := q.Index(c); got != want {
			t.Errorf("%v: got %d, want %d", c, got, want)
		}
	}
	for n := 16; n < 256; n++ {
		if got := q.Index(indexedRGB(n)); int(got) != n {
			t.Errorf("index %d %v: got %d", n, indexedRGB(n), got)
		}
	}
	for v := 8; v <= 238; v += 10 {
		c := art.RGB{uint8(v), uint8(v), uint8(v)}
		if got, want := q.Index(c), uint8(232+(v-8)/10); got != want {
			t.Errorf("grey %d: got %d, want %d", v, got, want)
		}
	}
}

func TestQuantiserMatchesBruteForce(t *testing.T) {
	q := NewQuantiser(colorprofile.ANSI256)
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		c := art.RGB{uint8(rng.IntN(256)), uint8(rng.IntN(256)), uint8(rng.IntN(256))}
		if got, want := q.Index(c), referenceNearest(c); got != want {
			t.Errorf("%v: got %d, want %d", c, got, want)
		}
	}
}

func referenceNearest(c art.RGB) uint8 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	lab := func(c [3]float64) [3]float64 {
		r, g, b := lin(uint8(c[0])), lin(uint8(c[1])), lin(uint8(c[2]))
		l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
		m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
		s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
		return [3]float64{0.2104542553*l + 0.7936177850*m - 0.0040720468*s, 1.9779984951*l - 2.4285922050*m + 0.4505937099*s, 0.0259040371*l + 0.7827717662*m - 0.8086757660*s}
	}
	lv := []float64{0, 95, 135, 175, 215, 255}
	var pal [256][3]float64
	for i := 16; i < 232; i++ {
		n := i - 16
		pal[i] = [3]float64{lv[n/36], lv[(n/6)%6], lv[n%6]}
	}
	for i := 232; i < 256; i++ {
		v := float64(8 + 10*(i-232))
		pal[i] = [3]float64{v, v, v}
	}
	x := lab([3]float64{float64(c[0]), float64(c[1]), float64(c[2])})
	best, bd := 16, math.Inf(1)
	for i := 16; i < 256; i++ {
		y := lab(pal[i])
		if d := math.Sqrt((x[0]-y[0])*(x[0]-y[0]) + (x[1]-y[1])*(x[1]-y[1]) + (x[2]-y[2])*(x[2]-y[2])); d < bd {
			best, bd = i, d
		}
	}
	return uint8(best)
}

func TestQuantiserOnlyFor256(t *testing.T) {
	for _, p := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI, colorprofile.ASCII, colorprofile.NoTTY} {
		if NewQuantiser(p) != nil {
			t.Errorf("%v: got a quantiser", p)
		}
	}
	var q *Quantiser
	s := "\x1b[38;2;1;2;3mx\x1b[0m"
	if got := q.Apply(s); got != s {
		t.Errorf("nil quantiser changed %q to %q", s, got)
	}
}

func TestQuantiserApply(t *testing.T) {
	q := NewQuantiser(colorprofile.ANSI256)
	for _, c := range []struct{ in, want string }{
		{"\x1b[38;2;44;49;56m▀\x1b[48;2;38;42;48m \x1b[0m", "\x1b[38;5;236m▀\x1b[48;5;235m \x1b[0m"},
		{"\x1b[1;48;2;255;0;0;38;2;0;0;255mX", "\x1b[1;48;5;196;38;5;21mX"},
		{"\x1b[38;2;255;255;255;mX", "\x1b[38;5;231;mX"},
		{"\x1b[38;5;100m\x1b[0m\x1b[?25h\x1b[2J", "\x1b[38;5;100m\x1b[0m\x1b[?25h\x1b[2J"},
		{"\x1b]8;;https://x/8;2;1\x1b\\a\x1b]8;;\x1b\\", "\x1b]8;;https://x/8;2;1\x1b\\a\x1b]8;;\x1b\\"},
		{"\x1b[38;2;300;0;0m\x1b[38;2;1;2m\x1b[38;2;1;2;3", "\x1b[38;2;300;0;0m\x1b[38;2;1;2m\x1b[38;2;1;2;3"},
		{"price 8;2;1;2 ZEC", "price 8;2;1;2 ZEC"},
		{"", ""},
	} {
		if got := q.Apply(c.in); got != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}
