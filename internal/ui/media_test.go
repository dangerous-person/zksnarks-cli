package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/audio"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/media"
)

func mediaModel(t *testing.T, withMedia bool, w, h int) *Model {
	t.Helper()
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	content, err := embedded.LoadSite()
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Atlas: a, Site: content, Client: market.New(true)}
	if withMedia {
		o.Media = media.FS
	}
	m := New(o)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func plainView(m *Model) []string {
	s := m.View().Content
	for strings.Contains(s, "\x1b[") {
		i := strings.Index(s, "\x1b[")
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		s = s[:i] + s[min(len(s), j+1):]
	}
	return strings.Split(s, "\n")
}

func TestThemeHintWithoutMedia(t *testing.T) {
	for _, withMedia := range []bool{false, true} {
		lines := plainView(mediaModel(t, withMedia, 160, 48))
		last := lines[len(lines)-1]
		if !strings.Contains(last, "[T] THEME") || strings.Contains(last, "[ENTER] PLAY") != withMedia {
			t.Errorf("media %v: %q", withMedia, last)
		}
	}
}

func joshSpot(t *testing.T, m *Model) int {
	t.Helper()
	for i, s := range m.opts.Site.Spotlights {
		if strings.Contains(s.Video, "josh") {
			return i
		}
	}
	t.Fatal("no interview with Josh")
	return 0
}

func TestPlayerHeldWhileSmall(t *testing.T) {
	m := mediaModel(t, true, 160, 48)
	m.setSection(secNetwork)
	m.spot = joshSpot(t, m)
	m.openPlayer()
	if m.watch.state != watchPlaying {
		t.Fatalf("state %v after opening", m.watch.state)
	}
	m.watch.at, m.watch.base = 20*time.Second, 20*time.Second
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.watch.state != watchPaused || !m.watch.held {
		t.Fatalf("state %v, held %v in a small window", m.watch.state, m.watch.held)
	}
	at := m.watch.at
	m.Update(watchTickMsg(m.watch.gen))
	m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if m.watch.state != watchPaused || m.watch.at != at {
		t.Errorf("moved on while held: %v at %v", m.watch.state, m.watch.at)
	}
	lines := plainView(m)
	if last := lines[len(lines)-1]; strings.Contains(last, "PAUSE") || !strings.Contains(last, "[ESC] CLOSE") {
		t.Errorf("hints while held: %q", last)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "MAKE THE WINDOW BIGGER_") {
		t.Error("no note to make the window bigger")
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if m.watch.state != watchPlaying || m.watch.held || m.watch.at != at {
		t.Errorf("state %v, held %v, at %v after growing", m.watch.state, m.watch.held, m.watch.at)
	}
}

type deadSound struct{ playing bool }

func (d *deadSound) Play()                   { d.playing = true }
func (d *deadSound) Pause()                  { d.playing = false }
func (d *deadSound) Rewind()                 { d.playing = false }
func (d *deadSound) Playing() bool           { return d.playing }
func (d *deadSound) Position() time.Duration { return 0 }
func (d *deadSound) Err() error              { return errors.New("the sound stopped") }

func TestInterviewWithoutSound(t *testing.T) {
	m := mediaModel(t, true, 160, 48)
	m.setSection(secNetwork)
	m.spot = joshSpot(t, m)
	c := m.interviewOf(m.spot)
	c.sound = &deadSound{}
	m.openPlayer()
	start := time.Now()
	m.clock(start)
	if c.sound != nil || c.soundErr == nil {
		t.Fatal("the dead sound is kept")
	}
	if got := m.clock(start.Add(1500 * time.Millisecond)); got < time.Second {
		t.Errorf("the clock stands at %v", got)
	}
	if !strings.HasPrefix(m.playerTime(m.spot, c), "no sound · ") {
		t.Errorf("time %q", m.playerTime(m.spot, c))
	}
}

func TestTrailerWithoutSound(t *testing.T) {
	m := mediaModel(t, true, 160, 48)
	m.montageRead()
	m.mont.sound = &deadSound{}
	m.openMontage()
	if view := strings.Join(plainView(m), "\n"); strings.Contains(view, m.mont.title) != (audio.Err() == nil) {
		t.Fatalf("title %q shown %v, audio error %v", m.mont.title, strings.Contains(view, m.mont.title), audio.Err())
	}
	start := time.Now()
	m.montageClock(start)
	if got := m.montageClock(start.Add(1500 * time.Millisecond)); got < m.mont.lead+time.Second {
		t.Errorf("the clock stands at %v", got)
	}
	view := strings.Join(plainView(m), "\n")
	if strings.Contains(view, m.mont.title) || !strings.Contains(view, "no sound") {
		t.Error("the title is not replaced by no sound")
	}
}

func TestAboutClipHeldWhileSmall(t *testing.T) {
	m := mediaModel(t, true, 160, 48)
	m.setSection(secAbout)
	m.View()
	if m.contactPic == (rect{}) {
		t.Fatal("no clip on the about page")
	}
	m.contactKey("space")
	tick := func(n int) {
		for range n {
			m.onContactTick()
		}
	}
	tick(3)
	at := m.contactFrame
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m.View()
	if m.contactPic != (rect{}) {
		t.Fatal("the clip still fits")
	}
	tick(5)
	m.contactKey("space")
	if m.contactFrame != at || !m.contactPlaying {
		t.Errorf("frame %d, want %d; playing %v", m.contactFrame, at, m.contactPlaying)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
	m.View()
	tick(1)
	if m.contactFrame != (at+1)%m.contactClip.Frames() {
		t.Errorf("frame %d after growing, want %d", m.contactFrame, at+1)
	}
}
