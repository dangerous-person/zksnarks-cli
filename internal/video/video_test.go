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

func TestDamaged(t *testing.T) {
	good := stream(2, 2, []art.RGB{{}}, "", 1, []byte{0, 0, 0, 0})
	for name, data := range map[string][]byte{
		"not gzip":  []byte("ZKV1"),
		"magic":     zip([]byte("ZKV2" + string(good[4:]))),
		"truncated": zip(good[:len(good)-1]),
		"odd":       zip(stream(2, 3, []art.RGB{{}}, "", 1, []byte{0, 0, 0, 0, 0, 0})),
		"sizes":     zip(good, stream(3, 2, []art.RGB{{}}, "", 1, []byte{0, 0, 0, 0, 0, 0})),
		"empty":     zip(),
	} {
		if _, err := Read(data); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
