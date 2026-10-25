package video

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// stream builds one ZKV1 (symbols == "") or ZKC1 stream at 4 fps.
func stream(w, h int, palette []art.RGB, symbols string, keyEvery int, frames ...[]byte) []byte {
	var b bytes.Buffer
	magic := "ZKV1"
	if symbols != "" {
		magic = "ZKC1"
	}
	b.WriteString(magic)
	for _, v := range []uint16{uint16(w), uint16(h), 400} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	binary.Write(&b, binary.LittleEndian, uint32(len(frames)))
	binary.Write(&b, binary.LittleEndian, uint16(keyEvery))
	binary.Write(&b, binary.LittleEndian, uint16(len(palette)))
	for _, c := range palette {
		b.Write(c[:])
	}
	if symbols != "" {
		b.WriteByte(byte(len(symbols)))
		b.WriteString(symbols)
	}
	for _, f := range frames {
		b.Write(f)
	}
	return b.Bytes()
}

func filled(fill byte, zkc1 []byte, palette int, symbols string) []byte {
	head := 18 + palette*3 + 1 + len(symbols)
	out := append([]byte("ZKC2"), zkc1[4:head]...)
	return append(append(out, fill), zkc1[head:]...)
}

func zip(streams ...[]byte) []byte {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	for _, s := range streams {
		z.Write(s)
	}
	z.Close()
	return b.Bytes()
}

// cell is what a terminal shows in one cell: the character and its colours, "" for the
// terminal's own.
type cell struct{ ch, fg, bg string }

// cells reads rendered rows back into cells.
func cells(t *testing.T, rows []string) [][]cell {
	t.Helper()
	var out [][]cell
	for _, r := range rows {
		var line []cell
		var fg, bg string
		for len(r) > 0 {
			if strings.HasPrefix(r, "\x1b[") {
				end := strings.IndexByte(r, 'm')
				code := r[2:end]
				switch {
				case code == "0":
					fg, bg = "", ""
				case code == "39":
					fg = ""
				case code == "49":
					bg = ""
				case strings.HasPrefix(code, "38;2;"):
					fg = code[5:]
				case strings.HasPrefix(code, "48;2;"):
					bg = code[5:]
				default:
					t.Fatalf("unexpected code %q", code)
				}
				r = r[end+1:]
				continue
			}
			ch := string([]rune(r)[0])
			c := cell{ch, fg, bg}
			if ch == " " {
				c.fg = ""
			}
			line = append(line, c)
			r = r[len(ch):]
		}
		out = append(out, line)
	}
	return out
}

func TestFrames(t *testing.T) {
	red, blue := art.RGB{255, 0, 0}, art.RGB{0, 0, 255}
	// half-blocks, 2x4 pixels in 2x2 cells, a keyframe every 2 frames
	blocks := stream(2, 4, []art.RGB{red, blue}, "", 2,
		[]byte{0, 0, 0, 1, 0, 0, 1, 1},                      // rr rb / rr bb
		[]byte{same, 1, same, same, same, same, same, same}, // rb rb / rr bb
		[]byte{1, 1, 1, 1, 0, 0, 0, 0},                      // bb bb / rr rr
	)
	// symbols " #" in 2x2 cells
	symbols := stream(2, 2, []art.RGB{{}, blue}, " #", 4,
		[]byte{0, 0, 1, 1, 1, 1, 0, 0},       // " #" / "# "
		[]byte{same, 0, 0, 0, same, 0, 1, 1}, // "  " / "##"
	)
	v, err := Read(zip(blocks, symbols))
	if err != nil {
		t.Fatal(err)
	}
	if c, r := v.Size(); c != 2 || r != 2 || v.Frames() != 5 || v.Duration() != 1250*time.Millisecond {
		t.Fatalf("size %dx%d, %d frames, %v", c, r, v.Frames(), v.Duration())
	}
	if got := v.At(1100 * time.Millisecond); got != 4 {
		t.Errorf("At(1.1s) = %d, want 4", got)
	}
	if got := v.At(time.Hour); got != 4 {
		t.Errorf("At(1h) = %d, want the last frame", got)
	}
	const r, b = "255;0;0", "0;0;255"
	R, B, rb := cell{" ", "", r}, cell{" ", "", b}, cell{"▀", r, b}
	none, hash := cell{" ", "", ""}, cell{"#", b, ""}
	cases := []struct {
		i    int
		g    art.Glyph
		want [][]cell
	}{
		{0, art.UpperHalf, [][]cell{{R, rb}, {rb, rb}}},
		{1, art.UpperHalf, [][]cell{{R, B}, {rb, rb}}},
		{2, art.UpperHalf, [][]cell{{B, B}, {R, R}}},
		{3, art.UpperHalf, [][]cell{{none, hash}, {hash, none}}},
		{4, art.UpperHalf, [][]cell{{none, none}, {hash, hash}}},
		{1, art.UpperHalf, [][]cell{{R, B}, {rb, rb}}}, // back from the other scene
		{0, art.LowerFiveEighths, [][]cell{{R, {"▅", b, r}}, {{"▅", b, r}, {"▅", b, r}}}},
	}
	for _, c := range cases {
		got := cells(t, v.Frame(c.i, c.g))
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("frame %d: %v, want %v", c.i, got, c.want)
		}
	}
}

func TestFill(t *testing.T) {
	orange := art.RGB{200, 100, 50}
	pal := []art.RGB{{}, orange}
	v, err := Read(zip(filled(128, stream(2, 1, pal, " #", 1, []byte{1, 1, 0, 0}), len(pal), " #")))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]cell{{{"#", "200;100;50", "100;50;25"}, {" ", "", ""}}}
	if got := cells(t, v.Frame(0, art.UpperHalf)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDamaged(t *testing.T) {
	good := stream(2, 2, []art.RGB{{}}, "", 1, []byte{0, 0, 0, 0})
	for name, data := range map[string][]byte{
		"not gzip":  []byte("ZKV1"),
		"magic":     zip([]byte("ZKV2" + string(good[4:]))),
		"truncated": zip(good[:len(good)-1]),
		"odd":       zip(stream(2, 3, []art.RGB{{}}, "", 1, []byte{0, 0, 0, 0, 0, 0})),
		"sizes":     zip(good, stream(3, 2, []art.RGB{{}}, "", 1, []byte{0, 0, 0, 0, 0, 0})),
		"empty":     zip(),
		"no fill":   zip(append([]byte("ZKC2"), stream(1, 1, []art.RGB{{}}, " ", 1, []byte{0, 0})[4:]...)),
	} {
		if _, err := Read(data); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestLimits(t *testing.T) {
	pal := []art.RGB{{}}
	frames := func(n, size int) [][]byte {
		out := make([][]byte, n)
		for i := range out {
			out[i] = make([]byte, size)
		}
		return out
	}
	fps := func(hundredths uint16, s []byte) []byte {
		s = bytes.Clone(s)
		binary.LittleEndian.PutUint16(s[8:], hundredths)
		return s
	}
	member := func(level int, data []byte) []byte {
		var b bytes.Buffer
		z, _ := gzip.NewWriterLevel(&b, level)
		z.Write(data)
		z.Close()
		return b.Bytes()
	}
	big := func(level, n int) []byte {
		out := member(level, stream(512, 512, pal, "", 1, frames(n, 0)...))
		return append(out, bytes.Repeat(member(level, make([]byte, 512*512)), n)...)
	}
	small := stream(1, 2, pal, "", 1, make([]byte, 2))
	ok := map[string][]byte{
		"widest and tallest":          zip(stream(512, 512, pal, "", 1, make([]byte, 512*512))),
		"widest and tallest, symbols": zip(stream(512, 256, pal, " ", 1, make([]byte, 512*256*2))),
		"most frames":                 zip(stream(1, 2, pal, "", 100, frames(20000, 2)...)),
		"most frames in scenes":       zip(stream(1, 2, pal, "", 100, frames(10000, 2)...), stream(1, 2, pal, "", 100, frames(10000, 2)...)),
		"slowest":                     zip(fps(100, small)),
		"fastest":                     zip(fps(12000, small)),
		"biggest file":                big(gzip.NoCompression, 255),
	}
	for name, data := range ok {
		if _, err := Read(data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string][]byte{
		"too wide":           zip(stream(513, 2, pal, "", 1, make([]byte, 513*2))),
		"too tall":           zip(stream(1, 514, pal, "", 1, make([]byte, 514))),
		"too tall, symbols":  zip(stream(1, 257, pal, " ", 1, make([]byte, 257*2))),
		"too many frames":    zip(stream(1, 2, pal, "", 100, frames(20001, 2)...)),
		"too many in scenes": zip(stream(1, 2, pal, "", 100, frames(10001, 2)...), stream(1, 2, pal, "", 100, frames(10000, 2)...)),
		"huge frame count":   zip(append(stream(1, 2, pal, "", 1)[:10:10], 0xff, 0xff, 0xff, 0xff, 1, 0, 1, 0, 0, 0, 0)),
		"no frame rate":      zip(fps(0, small)),
		"too slow":           zip(fps(99, small)),
		"too fast":           zip(fps(12001, small)),
		"no height":          zip(stream(2, 0, pal, "", 1, nil)),
		"file too big":       big(gzip.NoCompression, 257),
		"decodes too big":    big(gzip.BestSpeed, 1025),
	}
	for name, data := range bad {
		if _, err := Read(data); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
