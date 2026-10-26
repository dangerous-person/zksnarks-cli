//go:build !noaudio

// Package audio plays the sound of the spotlight interviews: MP3 decoded in Go and played
// through the system's audio (Core Audio, WASAPI or PulseAudio; no cgo). Its position is
// what the speakers are playing, so a video can follow it.
package audio

import (
	"bytes"
	"errors"
	"fmt"
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

const (
	startLimit = 2 * time.Second
	stallLimit = 2 * time.Second
	probeBytes = bytesPerSecond
	restAfter  = 300 * time.Millisecond
)

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
	mu   sync.Mutex
	ctx  *oto.Context
	err  error
}

type idler struct {
	mu  sync.Mutex
	dev interface {
		Suspend() error
		Resume() error
	}
	playing int
	asleep  bool
	gen     int
}

var idle idler

func (d *idler) wake() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.playing++
	d.gen++
	if d.asleep {
		d.dev.Resume()
		d.asleep = false
	}
}

func (d *idler) rest() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.playing--
	d.later()
}

func (d *idler) later() {
	if d.playing > 0 || d.asleep || d.dev == nil {
		return
	}
	d.gen++
	gen := d.gen
	time.AfterFunc(restAfter, func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.gen == gen && d.playing == 0 && !d.asleep {
			d.dev.Suspend()
			d.asleep = true
		}
	})
}

// open starts the audio device the first time a track needs it.
func open() (*oto.Context, error) {
	device.once.Do(func() {
		ctx, err := start()
		device.mu.Lock()
		device.ctx, device.err = ctx, err
		device.mu.Unlock()
		if ctx != nil {
			idle.mu.Lock()
			idle.dev = ctx
			idle.later()
			idle.mu.Unlock()
		}
	})
	device.mu.Lock()
	defer device.mu.Unlock()
	return device.ctx, device.err
}

func start() (*oto.Context, error) {
	type result struct {
		ctx *oto.Context
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{nil, fmt.Errorf("audio: %v", r)}
			}
		}()
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: rate, ChannelCount: 2, Format: oto.FormatSignedInt16LE})
		if err != nil {
			done <- result{nil, err}
			return
		}
		<-ready
		if err := ctx.Err(); err != nil {
			done <- result{nil, err}
			return
		}
		done <- result{ctx, nil}
	}()
	select {
	case r := <-done:
		return r.ctx, r.err
	case <-time.After(startLimit):
		return nil, errors.New("audio: the device did not start")
	}
}

func Err() error {
	device.mu.Lock()
	defer device.mu.Unlock()
	if device.err != nil || device.ctx == nil {
		return device.err
	}
	return device.ctx.Err()
}

// Track is one sound, paused at the start until Play.
type Track struct {
	player  *oto.Player
	src     *source
	length  time.Duration
	playing bool
	dog     watchdog
}

type watchdog struct {
	heard  int64
	moved  time.Time
	polled time.Time
}

func (w *watchdog) start(now time.Time) { w.moved, w.polled = now, now }

func (w *watchdog) stuck(heard int64, now time.Time) bool {
	gap := now.Sub(w.polled)
	w.polled = now
	if heard != w.heard || gap > stallLimit/2 {
		w.heard, w.moved = heard, now
		return false
	}
	return now.Sub(w.moved) > stallLimit
}

// source is the decoded sound from its first real sample, counting the bytes the player
// has taken.
type source struct {
	mu   sync.Mutex
	dec  io.ReadSeeker
	skip int64 // bytes of encoder padding before the first real sample
	n    int64
	err  error
}

func (s *source) Read(p []byte) (n int, err error) {
	defer s.catch(&err)
	n, err = s.dec.Read(p)
	s.mu.Lock()
	s.n += int64(n)
	if err != nil && err != io.EOF {
		s.err = err
	}
	s.mu.Unlock()
	return n, err
}

// Seek only supports io.SeekStart, which is all the player uses.
func (s *source) Seek(offset int64, whence int) (pos int64, err error) {
	defer s.catch(&err)
	if whence != io.SeekStart {
		return 0, errors.New("audio: unsupported seek")
	}
	pos, err = s.dec.Seek(s.skip+offset, io.SeekStart)
	if err != nil {
		s.fail(err)
		return 0, err
	}
	s.mu.Lock()
	s.n = pos - s.skip
	s.mu.Unlock()
	return pos - s.skip, nil
}

func (s *source) catch(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("audio: damaged sound: %v", r)
		s.fail(*err)
	}
}

func (s *source) fail(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func (s *source) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *source) taken() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// Load decodes an MP3 (48 kHz) and opens the audio device for it.
func Load(data []byte) (*Track, error) {
	src, length, err := decode(data)
	if err != nil {
		return nil, err
	}
	ctx, err := open()
	if err != nil {
		return nil, err
	}
	return &Track{player: ctx.NewPlayer(src), src: src, length: length}, nil
}

func decode(data []byte) (src *source, length time.Duration, err error) {
	defer func() {
		if r := recover(); r != nil {
			src, length, err = nil, 0, fmt.Errorf("audio: damaged sound: %v", r)
		}
	}()
	dec, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if dec.SampleRate() != rate {
		return nil, 0, errors.New("audio: the sound must be 48 kHz")
	}
	src = &source{dec: dec, skip: int64(padding(data)) * 4}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	if _, err := io.ReadFull(src, make([]byte, probeBytes)); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = errors.New("audio: the sound is too short")
		}
		return nil, 0, err
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	return src, time.Duration(dec.Length()-src.skip) * time.Second / bytesPerSecond, nil
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
	if lame+24 <= len(data) {
		switch string(data[lame : lame+4]) {
		case "LAME", "Lavc", "Lavf":
			delay := int(data[lame+21])<<4 | int(data[lame+22])>>4
			samples += delay + 529
		}
	}
	return samples
}

// Play starts or resumes the sound.
func (t *Track) Play() {
	if !t.playing {
		idle.wake()
	}
	t.player.Play()
	t.playing = true
	t.dog.start(time.Now())
}

// Pause holds the sound where it is.
func (t *Track) Pause() {
	t.player.Pause()
	if t.playing {
		idle.rest()
	}
	t.playing = false
}

// Rewind pauses the sound and goes back to the start.
func (t *Track) Rewind() {
	t.Pause()
	t.player.Seek(0, io.SeekStart)
}

func (t *Track) Seek(d time.Duration) {
	t.Pause()
	off := int64(d) * bytesPerSecond / int64(time.Second)
	t.player.Seek(off-off%4, io.SeekStart)
}

func (t *Track) Err() error {
	if err := t.src.failure(); err != nil {
		return err
	}
	if err := t.player.Err(); err != nil {
		return err
	}
	if err := Err(); err != nil {
		return err
	}
	if t.playing && t.player.IsPlaying() && t.dog.stuck(t.src.taken()-int64(t.player.BufferedSize()), time.Now()) {
		return errors.New("audio: the sound stopped")
	}
	return nil
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
func (t *Track) Close() {
	if t.playing {
		idle.rest()
	}
	t.playing = false
	t.player.PauseAndStopReading()
}
