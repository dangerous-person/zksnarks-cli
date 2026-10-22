// Package audio plays the sound of the spotlight interviews: MP3 decoded in Go and played
// through the system's audio (Core Audio, WASAPI or PulseAudio; no cgo). Its position is
// what the speakers are playing, so a video can follow it.
package audio

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

// rate is the sample rate the audio device is opened at; every track must have it.
const rate = 48000

const bytesPerSecond = rate * 4 // 16-bit stereo

// latency is how far the device's own buffers run ahead of the speakers with oto's
// defaults: four 32 ms buffers on macOS, 50 ms on Windows, 100 ms with PulseAudio. A track's
// position is what has left the player minus this.
var latency = map[string]time.Duration{"darwin": 115 * time.Millisecond, "windows": 45 * time.Millisecond}[runtime.GOOS]

func init() {
	if latency == 0 {
		latency = 100 * time.Millisecond
	}
}

var device struct {
	once sync.Once
	ctx  *oto.Context
	err  error
}

// open starts the audio device the first time a track needs it.
func open() (*oto.Context, error) {
	device.once.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: rate, ChannelCount: 2, Format: oto.FormatSignedInt16LE})
		if err != nil {
			device.err = err
			return
		}
		<-ready
		device.ctx = ctx
	})
	return device.ctx, device.err
}

// Track is one sound, paused at the start until Play.
type Track struct {
	player *oto.Player
	src    *source
	length time.Duration
}

// source is the decoded sound from its first real sample, counting the bytes the player
// has taken.
type source struct {
	mu   sync.Mutex
	dec  *mp3.Decoder
	skip int64 // bytes of encoder padding before the first real sample
	n    int64
}

func (s *source) Read(p []byte) (int, error) {
	n, err := s.dec.Read(p)
	s.mu.Lock()
	s.n += int64(n)
	s.mu.Unlock()
	return n, err
}

// Seek only supports io.SeekStart, which is all the player uses.
func (s *source) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart {
		return 0, errors.New("audio: unsupported seek")
	}
	pos, err := s.dec.Seek(s.skip+offset, io.SeekStart)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.n = pos - s.skip
	s.mu.Unlock()
	return s.n, nil
}

func (s *source) taken() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// Load decodes an MP3 (48 kHz) and opens the audio device for it.
func Load(data []byte) (*Track, error) {
	dec, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if dec.SampleRate() != rate {
		return nil, errors.New("audio: the sound must be 48 kHz")
	}
	ctx, err := open()
	if err != nil {
		return nil, err
	}
	src := &source{dec: dec, skip: int64(padding(data)) * 4}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	length := time.Duration(dec.Length()-src.skip) * time.Second / bytesPerSecond
	return &Track{player: ctx.NewPlayer(src), src: src, length: length}, nil
}

// padding is the number of samples the decoder puts before the encoder's first one: the
// Xing or Info frame, which it decodes as silence, then LAME's encoder delay from the tag
// in that frame and the 529 samples every MP3 decoder lags behind. ffmpeg's encoder writes
// a 576 sample delay, 2257 samples (47 ms) in all.
func padding(data []byte) int {
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		size := int(data[6]&0x7f)<<21 | int(data[7]&0x7f)<<14 | int(data[8]&0x7f)<<7 | int(data[9]&0x7f)
		data = data[min(len(data), 10+size):]
	}
	if len(data) < 4 || data[0] != 0xff || data[1]&0xe0 != 0xe0 {
		return 0
	}
	head := data[:min(len(data), 48)] // the tag follows the frame header and side info
	tag := max(bytes.Index(head, []byte("Xing")), bytes.Index(head, []byte("Info")))
	if tag < 0 || tag+8 > len(data) {
		return 0
	}
	samples := 1152
	lame := tag + 8
	for bit, size := range []int{4, 4, 100, 4} { // frames, bytes, table of contents, quality
		if data[tag+7]&(1<<bit) != 0 {
			lame += size
		}
	}
	if lame+24 <= len(data) && (string(data[lame:lame+4]) == "LAME" || string(data[lame:lame+4]) == "Lavc") {
		delay := int(data[lame+21])<<4 | int(data[lame+22])>>4
		samples += delay + 529
	}
	return samples
}

// Play starts or resumes the sound.
func (t *Track) Play() { t.player.Play() }

// Pause holds the sound where it is.
func (t *Track) Pause() { t.player.Pause() }

// Rewind pauses the sound and goes back to the start.
func (t *Track) Rewind() {
	t.player.Pause()
	t.player.Seek(0, io.SeekStart)
}

// Playing reports whether the sound is playing; it stops by itself at the end.
func (t *Track) Playing() bool { return t.player.IsPlaying() }

// Position is the moment of the sound the speakers are playing.
func (t *Track) Position() time.Duration {
	var n int64
	var buffered int
	for range 3 { // the player may take more from the source between the two reads
		n = t.src.taken()
		buffered = t.player.BufferedSize()
		if t.src.taken() == n {
			break
		}
	}
	d := time.Duration(n-int64(buffered)) * time.Second / bytesPerSecond
	if t.player.IsPlaying() {
		d -= latency
	}
	return max(0, min(d, t.length))
}

// Length is how long the sound is.
func (t *Track) Length() time.Duration { return t.length }

// Close stops the sound for good.
func (t *Track) Close() { t.player.Close() }
