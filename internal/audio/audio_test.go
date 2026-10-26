//go:build !noaudio

package audio

import (
	"bytes"
	"io"
	"io/fs"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/media"
)

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
		{"ffmpeg, bitexact", frame(false, "Info", "Lavf lame", 0x0f), 2257},
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

func TestDecode(t *testing.T) {
	names, _ := fs.Glob(media.FS, "*.mp3")
	more, _ := fs.Glob(media.FS, "trailer/*.mp3")
	names = append(names, more...)
	if len(names) != 4 {
		t.Fatalf("%d sounds: %v", len(names), names)
	}
	for _, name := range names {
		data, err := fs.ReadFile(media.FS, name)
		if err != nil {
			t.Fatal(err)
		}
		src, length, err := decode(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if length < 30*time.Second || src.taken() != 0 {
			t.Errorf("%s: %v long, %d bytes taken", name, length, src.taken())
		}
	}
}

func TestDamagedSound(t *testing.T) {
	josh, err := fs.ReadFile(media.FS, "josh-spotlight.mp3")
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	noise := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Uint32())
		}
		return b
	}
	cases := map[string][]byte{
		"empty":      nil,
		"not an MP3": []byte("RIFF....WAVE"),
		"too short":  josh[:4<<10],
	}
	for i := range 20 {
		cases["noise "+string(rune('a'+i))] = noise(20 << 10)
	}
	for name, data := range cases {
		if _, _, err := decode(data); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	bad := bytes.Clone(josh)
	bad[27648] = 0x92
	src, _, err := decode(bad)
	if err != nil {
		t.Fatalf("the first second is fine: %v", err)
	}
	if _, err := io.Copy(io.Discard, src); err == nil || src.failure() == nil {
		t.Errorf("read on past a broken frame: %v", err)
	}
}

type panicky struct{}

func (panicky) Read([]byte) (int, error) { panic("bad frame") }

func (panicky) Seek(int64, int) (int64, error) { panic("bad frame") }

func TestPanickingDecoder(t *testing.T) {
	s := &source{dec: panicky{}}
	if n, err := s.Read(make([]byte, 16)); n != 0 || err == nil {
		t.Errorf("Read: %d, %v", n, err)
	}
	if _, err := s.Seek(0, io.SeekStart); err == nil {
		t.Error("Seek: no error")
	}
	if s.failure() == nil {
		t.Error("no failure noted")
	}
}

func TestWatchdog(t *testing.T) {
	const step = 40 * time.Millisecond
	t0 := time.Now()
	at := func(i int) time.Time { return t0.Add(time.Duration(i) * step) }
	var w watchdog
	w.start(t0)
	for i := 1; i <= 100; i++ {
		if w.stuck(int64(i), at(i)) {
			t.Fatalf("stuck at %d while moving", i)
		}
	}
	stuck := 0
	for i := 101; i <= 200 && stuck == 0; i++ {
		if w.stuck(100, at(i)) {
			stuck = i - 100
		}
	}
	if d := time.Duration(stuck) * step; d <= stallLimit || d > stallLimit+step {
		t.Errorf("stuck after %v, want just over %v", d, stallLimit)
	}
	w.start(t0)
	w.stuck(7, at(1))
	if w.stuck(7, at(1).Add(5*time.Second)) || w.stuck(7, at(2).Add(5*time.Second)) {
		t.Error("stuck right after the program was held up")
	}
}

type fakeDevice struct {
	mu                sync.Mutex
	suspends, resumes int
}

func (f *fakeDevice) Suspend() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.suspends++
	return nil
}

func (f *fakeDevice) Resume() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumes++
	return nil
}

func (f *fakeDevice) calls() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.suspends, f.resumes
}

func TestIdleDeviceRests(t *testing.T) {
	dev := &fakeDevice{}
	d := &idler{dev: dev}
	asleep := func(suspends int) time.Duration {
		t.Helper()
		start := time.Now()
		for time.Since(start) < 2*time.Second {
			if s, _ := dev.calls(); s == suspends {
				return time.Since(start)
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("not suspended %d times", suspends)
		return 0
	}
	d.mu.Lock()
	d.later()
	d.mu.Unlock()
	if took := asleep(1); took < restAfter-10*time.Millisecond {
		t.Errorf("suspended after %v, want %v", took, restAfter)
	}
	d.wake()
	if _, r := dev.calls(); r != 1 {
		t.Fatalf("%d resumes after a play", r)
	}
	d.rest()
	d.wake()
	d.wake()
	d.rest()
	time.Sleep(restAfter + 200*time.Millisecond)
	if s, r := dev.calls(); s != 1 || r != 1 {
		t.Fatalf("%d suspends, %d resumes while a track plays", s, r)
	}
	d.rest()
	asleep(2)
	d.wake()
	if s, r := dev.calls(); s != 2 || r != 2 {
		t.Errorf("%d suspends, %d resumes", s, r)
	}
}
