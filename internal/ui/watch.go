package ui

import (
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/audio"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

// interview is a spotlight's interview from the media folder: the terminal video and its sound.
type interview struct {
	name     string
	video    *video.Video
	mp3      []byte // nil when there is no sound file
	sound    sound  // opened on the first watch
	loading  bool
	soundErr error
}

// sound is what the player needs of an *audio.Track.
type sound interface {
	Play()
	Pause()
	Rewind()
	Playing() bool
	Position() time.Duration
	Err() error
}

// interviewOf is the interview of spotlight i: NAME.zkv and NAME.mp3, NAME being the site's
// video file name. It is nil when there is no video; openInterview reads it.
func (m *Model) interviewOf(i int) *interview {
	if c, ok := m.interviews[i]; ok {
		return c
	}
	var c *interview
	if s := m.opts.Site.Spotlights; i < len(s) && s[i].Video != "" {
		name := strings.TrimSuffix(path.Base(s[i].Video), path.Ext(s[i].Video))
		if isMedia(m.opts.Media, name+".zkv") {
			c = &interview{name: name}
		}
	}
	m.interviews[i] = c
	return c
}

func (m *Model) openInterview(i int) *interview {
	c := m.interviewOf(i)
	if c == nil || c.video != nil {
		return c
	}
	data, err := readMedia(m.opts.Media, c.name+".zkv")
	if err == nil {
		c.video, err = video.Read(data)
	}
	if err != nil {
		m.interviews[i] = nil
		return nil
	}
	c.mp3, _ = readMedia(m.opts.Media, c.name+".mp3")
	return c
}

const mediaLimit = 64 << 20

func readMedia(media fs.FS, name string) ([]byte, error) {
	if media == nil {
		return nil, fs.ErrNotExist
	}
	f, err := media.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > mediaLimit {
		return nil, fs.ErrInvalid
	}
	data := make([]byte, st.Size())
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}
	return data, nil
}

func isMedia(media fs.FS, name string) bool {
	if media == nil {
		return false
	}
	st, err := fs.Stat(media, name)
	return err == nil && st.Mode().IsRegular() && st.Size() <= mediaLimit
}

type watchState int

const (
	watchStopped watchState = iota
	watchPlaying
	watchPaused
)

// watch is the interview being played on the network page. The position runs on the wall
// clock, which keeps the frames evenly paced, and follows the sound: the device reports it
// in steps of a few dozen milliseconds.
type watch struct {
	spot   int
	state  watchState
	at     time.Duration // the position on screen
	t0     time.Time     // the wall clock runs the position from base at t0
	base   time.Duration
	heard  time.Duration
	synced bool // the sound has reached the speakers, so the clock follows it
	gen    int  // ticks of an earlier run are dropped
	held   bool
}

// drift is how far the clock may stray from the sound before it jumps to it; smaller
// differences are evened out over a few frames.
const drift = 60 * time.Millisecond

type watchTickMsg int

type soundMsg struct {
	iv    *interview
	sound sound
	err   error
}

// watchKey handles the player's keys on the network page.
func (m *Model) watchKey(k string) (tea.Cmd, bool) {
	c := m.openInterview(m.spot)
	if c == nil {
		return nil, false
	}
	w := &m.watch
	if w.spot != m.spot {
		m.stopWatch()
		w.spot = m.spot
	}
	switch strings.ToLower(k) {
	case "w":
		if w.state == watchPlaying {
			return nil, true
		}
		return m.playWatch(c), true
	case "p":
		switch w.state {
		case watchPlaying:
			m.pauseWatch()
		case watchPaused:
			return m.playWatch(c), true
		}
		return nil, true
	case "s":
		m.stopWatch()
		return nil, true
	}
	return nil, false
}

// playWatch starts the interview, or resumes it, and opens its sound the first time.
func (m *Model) playWatch(c *interview) tea.Cmd {
	w := &m.watch
	if w.state == watchStopped {
		w.at = 0
	}
	if m.playerSmall(c) {
		w.state, w.held = watchPaused, true
		w.gen++
		return nil
	}
	w.state, w.base, w.t0, w.synced, w.held = watchPlaying, w.at, time.Now(), false, false
	w.gen++
	cmds := []tea.Cmd{m.watchTick()}
	if c.sound == nil && c.mp3 != nil && !c.loading && c.soundErr == nil {
		c.soundErr = audio.Err()
	}
	switch {
	case c.sound != nil:
		c.sound.Play()
	case c.mp3 != nil && !c.loading && c.soundErr == nil:
		c.loading = true
		cmds = append(cmds, func() tea.Msg {
			s, err := audio.Load(c.mp3)
			if err != nil {
				return soundMsg{c, nil, err}
			}
			return soundMsg{c, s, nil}
		})
	}
	return tea.Batch(cmds...)
}

// pauseWatch holds the interview; the picture lands where the sound stops.
func (m *Model) pauseWatch() {
	w := &m.watch
	if w.state != watchPlaying {
		return
	}
	w.at = m.clock(time.Now())
	if c := m.interviewOf(w.spot); c != nil && c.sound != nil {
		c.sound.Pause()
		if w.synced {
			w.at = c.sound.Position()
		}
	}
	w.state = watchPaused
	w.gen++
}

// stopWatch ends the interview and goes back to its first frame.
func (m *Model) stopWatch() {
	w := &m.watch
	if c := m.interviewOf(w.spot); c != nil && c.sound != nil {
		c.sound.Rewind()
	}
	w.state, w.at, w.held = watchStopped, 0, false
	w.gen++
}

func (m *Model) fitPlayer() tea.Cmd {
	if m.mont.open {
		return m.fitMontage()
	}
	w := &m.watch
	if !m.playerOpen() || w.spot != m.spot {
		return nil
	}
	c := m.interviewOf(m.spot)
	if c == nil {
		return nil
	}
	small := m.playerSmall(c)
	switch {
	case small && w.state == watchPlaying:
		m.pauseWatch()
		w.held = true
	case !small && w.held:
		return m.playWatch(c)
	}
	return nil
}

// watching is the playback state of spotlight i.
func (m *Model) watching(i int) (watchState, time.Duration) {
	if m.watch.spot != i {
		return watchStopped, 0
	}
	return m.watch.state, m.watch.at
}

// onSound starts the sound once it is open; without it the clip plays on the wall clock.
func (m *Model) onSound(msg soundMsg) {
	c := msg.iv
	c.loading, c.sound, c.soundErr = false, msg.sound, msg.err
	w := &m.watch
	if m.interviewOf(w.spot) != c {
		return
	}
	w.t0, w.base = time.Now(), w.at
	if c.sound != nil && w.state == watchPlaying {
		c.sound.Play()
	}
}

func (m *Model) onWatchTick(msg watchTickMsg) tea.Cmd {
	w := &m.watch
	if int(msg) != w.gen || w.state != watchPlaying {
		return nil
	}
	c := m.interviewOf(w.spot)
	pos := m.clock(time.Now())
	if pos >= c.video.Duration() {
		m.stopWatch()
		return nil
	}
	w.at = pos
	return m.watchTick()
}

// watchTick waits for the next frame.
func (m *Model) watchTick() tea.Cmd {
	w := &m.watch
	v := m.interviewOf(w.spot).video
	next := time.Duration(float64(v.At(w.at)+1)/v.FPS()*float64(time.Second)) - w.at
	gen := w.gen
	return tea.Tick(max(next, 4*time.Millisecond), func(time.Time) tea.Msg { return watchTickMsg(gen) })
}

// clock is the position at now while playing.
func (m *Model) clock(now time.Time) time.Duration {
	w := &m.watch
	c := m.interviewOf(w.spot)
	if w.state == watchPlaying && c.sound != nil {
		if err := c.sound.Err(); err != nil {
			if w.synced {
				w.base += now.Sub(w.t0)
			}
			c.sound.Pause()
			c.sound, c.soundErr, w.synced, w.t0 = nil, err, false, now
		}
	}
	switch {
	case w.state != watchPlaying:
		return w.at
	case c.loading:
		w.t0 = now // wait for the sound
		return w.base
	case c.sound == nil || w.synced && !c.sound.Playing(): // no sound, or it has ended
		return w.base + now.Sub(w.t0)
	}
	a := c.sound.Position()
	moved := a != w.heard
	w.heard = a
	if !w.synced {
		if a <= w.base { // not at the speakers yet
			w.t0 = now
			return w.base
		}
		w.synced, w.t0, w.base = true, now, a
		return a
	}
	pos := w.base + now.Sub(w.t0)
	if !moved {
		return pos
	}
	e := a - pos
	if e > drift || e < -drift {
		w.t0, w.base = now, a
		return a
	}
	w.base += e / 8
	return pos + e/8
}

// mmss formats a position as minutes and seconds.
func mmss(d time.Duration) string {
	s := int(d / time.Second)
	sec := itoa(s % 60)
	if s%60 < 10 {
		sec = "0" + sec
	}
	return itoa(s/60) + ":" + sec
}

// player draws the clip of spotlight i: the frame, a blank line and a progress bar.
func (m *Model) player(i int, c *interview) []line {
	_, at := m.watching(i)
	v := c.video
	var out []line
	cols, _ := v.Size()
	for _, l := range v.Frame(v.At(at), m.opts.Glyph) {
		out = append(out, line{l, cols})
	}
	return append(out, line{}, m.progress(at, v.Duration(), cols))
}

// playerKeys is the player's keys for its state, after the one that closes it.
func (m *Model) playerKeys(i int) line {
	pairs := []string{"esc", "close", "p", "play"}
	switch state, _ := m.watching(i); {
	case m.watch.held && m.watch.spot == i:
		pairs = pairs[:2]
	case state == watchPlaying:
		pairs = []string{"esc", "close", "p", "pause", "s", "stop"}
	case state == watchPaused:
		pairs = []string{"esc", "close", "p", "resume", "s", "stop"}
	}
	return m.hints(pairs...)
}

// playerTime is where the clip of spotlight i is and how long it is, noting a missing sound.
func (m *Model) playerTime(i int, c *interview) string {
	_, at := m.watching(i)
	t := mmss(at) + " / " + mmss(c.video.Duration())
	if c.soundErr != nil || c.mp3 == nil || audio.Err() != nil {
		t = "no sound · " + t
	}
	return t
}

// progress is the share of the clip played, in the colour of the window's title badge on a
// quieter line.
func (m *Model) progress(at, total time.Duration, w int) line {
	done := 0
	if total > 0 {
		done = min(w, int(int64(w)*int64(at)/int64(total)))
	}
	var r row
	r.put(fg(m.theme.ModalTitleBg), strings.Repeat("━", done))
	r.put(fg(mix(m.theme.ModalTitleBg, m.theme.ModalBg, 0.6)), strings.Repeat("─", w-done))
	return r.line()
}
