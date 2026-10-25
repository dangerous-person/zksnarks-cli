package term

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

type Quantiser struct {
	memo map[art.RGB]uint8
}

func NewQuantiser(p colorprofile.Profile) *Quantiser {
	if p != colorprofile.ANSI256 {
		return nil
	}
	return &Quantiser{memo: map[art.RGB]uint8{}}
}

func (q *Quantiser) Index(c art.RGB) uint8 {
	n, ok := q.memo[c]
	if !ok {
		n = nearest(c)
		q.memo[c] = n
	}
	return n
}

func (q *Quantiser) Apply(s string) string {
	if q == nil || !strings.Contains(s, "8;2;") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			break
		}
		j := i + 2
		for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';') {
			j++
		}
		if j == len(s) || s[j] != 'm' {
			b.WriteString(s[:j])
			s = s[j:]
			continue
		}
		b.WriteString(s[:i+2])
		q.params(&b, s[i+2:j])
		b.WriteByte('m')
		s = s[j+1:]
	}
	b.WriteString(s)
	return b.String()
}

func (q *Quantiser) params(b *strings.Builder, p string) {
	for i := 0; ; i++ {
		f, rest, more := strings.Cut(p, ";")
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(f)
		if (f == "38" || f == "48") && strings.HasPrefix(rest, "2;") {
			if c, n, ok := rgb(rest[2:]); ok {
				b.WriteString(";5;")
				b.WriteString(strconv.Itoa(int(q.Index(c))))
				rest, more = strings.CutPrefix(rest[2+n:], ";")
			}
		}
		if !more {
			return
		}
		p = rest
	}
}

func rgb(s string) (art.RGB, int, bool) {
	var c art.RGB
	i := 0
	for k := range c {
		if k > 0 {
			if i == len(s) || s[i] != ';' {
				return c, 0, false
			}
			i++
		}
		v, start := 0, i
		for i < len(s) && i-start < 3 && s[i] >= '0' && s[i] <= '9' {
			v = v*10 + int(s[i]-'0')
			i++
		}
		if i == start || v > 255 {
			return c, 0, false
		}
		c[k] = uint8(v)
	}
	if i < len(s) && s[i] != ';' {
		return c, 0, false
	}
	return c, i, true
}

var cubeLevels = [6]uint8{0, 95, 135, 175, 215, 255}

func indexedRGB(n int) art.RGB {
	if n >= 232 {
		v := uint8(8 + 10*(n-232))
		return art.RGB{v, v, v}
	}
	n -= 16
	return art.RGB{cubeLevels[n/36], cubeLevels[n/6%6], cubeLevels[n%6]}
}

var linear = func() (t [256]float64) {
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = c / 12.92
		} else {
			t[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return t
}()

func oklab(c art.RGB) [3]float64 {
	r, g, b := linear[c[0]], linear[c[1]], linear[c[2]]
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

var paletteLab = func() (t [256][3]float64) {
	for n := 16; n < 256; n++ {
		t[n] = oklab(indexedRGB(n))
	}
	return t
}()

func nearest(c art.RGB) uint8 {
	p := oklab(c)
	best, bd := 16, math.Inf(1)
	for n := 16; n < 256; n++ {
		q := &paletteLab[n]
		d0, d1, d2 := p[0]-q[0], p[1]-q[1], p[2]-q[2]
		if d := d0*d0 + d1*d1 + d2*d2; d < bd {
			best, bd = n, d
		}
	}
	return uint8(best)
}
