package audio

import "testing"

func TestPadding(t *testing.T) {
	frame := func(id3 bool, tag, encoder string, flags byte) []byte {
		var b []byte
		if id3 {
			b = append(b, "ID3\x04\x00\x00\x00\x00\x01\x0a"...) // 138 bytes of tag
			b = append(b, make([]byte, 138)...)
		}
		b = append(b, 0xff, 0xfb, 0x94, 0x00)
		b = append(b, make([]byte, 32)...) // side info
		b = append(b, tag...)
		b = append(b, 0, 0, 0, flags)
		for bit, size := range []int{4, 4, 100, 4} {
			if flags&(1<<bit) != 0 {
				b = append(b, make([]byte, size)...)
			}
		}
		lame := append([]byte(encoder), make([]byte, 30)...)
		lame[21], lame[22], lame[23] = 0x24, 0x05, 0xc0 // delay 576, padding 1472
		return append(append(b, lame...), make([]byte, 300)...)
	}
	cases := []struct {
		name string
		data []byte
		want int
	}{
		{"ffmpeg", frame(false, "Info", "Lavc62.28", 0x0f), 2257},
		{"with ID3", frame(true, "Info", "Lavc62.28", 0x0f), 2257},
		{"LAME, VBR", frame(false, "Xing", "LAME3.100", 0x0f), 2257},
		{"no table", frame(false, "Info", "LAME3.100", 0x0b), 2257},
		{"no encoder tag", frame(false, "Info", "Other", 0x0f), 1152},
		{"no info frame", frame(false, "Nope", "LAME3.100", 0x0f), 0},
		{"not an MP3", []byte("RIFF....WAVE"), 0},
		{"empty", nil, 0},
	}
	for _, c := range cases {
		if got := padding(c.data); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
