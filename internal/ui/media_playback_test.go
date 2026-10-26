package ui

import (
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/media"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

type fakeSound struct {
	at      time.Duration
	playing bool
}

func (f *fakeSound) Play()                   { f.playing = true }
func (f *fakeSound) Pause()                  { f.playing = false }
func (f *fakeSound) Rewind()                 { f.playing, f.at = false, 0 }
func (f *fakeSound) Playing() bool           { return f.playing }
func (f *fakeSound) Position() time.Duration { return f.at }
func (f *fakeSound) Err() error              { return nil }

func TestTrailerHeldWhileSmall(t *testing.T) {
	m := mediaModel(t, true, 160, 48)
	m.montageRead()
	s := &fakeSound{}
	m.mont.sound = s
	m.openMontage()
	if m.mont.state != watchPlaying || !s.playing {
		t.Fatalf("state %v, sound playing %v after opening", m.mont.state, s.playing)
	}
	m.mont.at, m.mont.base = 20*time.Second, 20*time.Second
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	if m.mont.state != watchPaused || !m.mont.held || s.playing {
		t.Fatalf("state %v, held %v, sound playing %v in a small window", m.mont.state, m.mont.held, s.playing)
	}
	at := m.mont.at
	m.Update(montageTickMsg(m.mont.gen))
	m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if m.mont.state != watchPaused || m.mont.at != at || s.playing {
		t.Errorf("moved on while held: %v at %v, sound playing %v", m.mont.state, m.mont.at, s.playing)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
	if m.mont.state != watchPlaying || m.mont.held || m.mont.at != at || !s.playing {
		t.Errorf("state %v, held %v, at %v, sound playing %v after growing", m.mont.state, m.mont.held, m.mont.at, s.playing)
	}
	m.closeMontage()
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	m.openMontage()
	if m.mont.state != watchPaused || !m.mont.held || s.playing {
		t.Errorf("opened in a small window: state %v, held %v, sound playing %v", m.mont.state, m.mont.held, s.playing)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
	if m.mont.state != watchPlaying || !s.playing {
		t.Errorf("state %v, sound playing %v after growing", m.mont.state, s.playing)
	}
}

func TestMontageHoldsShortGaps(t *testing.T) {
	data, err := fs.ReadFile(media.FS, "trailer/01-walker.zkv")
	if err != nil {
		t.Fatal(err)
	}
	v, err := video.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	p := &montagePlayer{items: []montageItem{
		{Start: 0, Len: 1, video: v},
		{Start: 1.02, In: 3, Len: 1, video: v},
		{Start: 2.3, Len: 1, video: v},
	}}
	at := func(s float64) (*video.Video, int) { return p.frameAt(time.Duration(s * float64(time.Second))) }
	_, last := at(0.999)
	if got, f := at(1.01); got != v || f != last {
		t.Errorf("20 ms gap: frame %d of %p, want the last one shown, %d", f, got, last)
	}
	for _, s := range []float64{2.02, 2.05, 2.2, 3.3, 4} {
		if got, f := at(s); got != nil {
			t.Errorf("at %vs: frame %d, want black", s, f)
		}
	}
	m := mediaModel(t, true, 160, 48)
	m.montageRead()
	m.montageDecode()
	items := m.mont.items
	for i, it := range items[:len(items)-1] {
		end, next := it.Start+it.Len, items[i+1].Start
		if next <= end {
			continue
		}
		if got, _ := m.mont.frameAt(time.Duration((end + next) / 2 * float64(time.Second))); (got == nil) != (next-end >= montageBridge) {
			t.Errorf("trailer: gap of %.0f ms at %.3fs black %v", (next-end)*1000, end, got == nil)
		}
	}
}

func TestClocksRunOnWhileTheSoundStalls(t *testing.T) {
	const step = time.Second / 24
	for _, name := range []string{"trailer", "interview"} {
		m := mediaModel(t, true, 160, 48)
		s := &fakeSound{playing: true}
		now := time.Unix(1000, 0)
		var clock func(time.Time) time.Duration
		if name == "trailer" {
			p := &m.mont
			p.sound, p.state, p.t0 = s, watchPlaying, now
			clock = m.montageClock
		} else {
			m.setSection(secNetwork)
			m.spot = joshSpot(t, m)
			m.interviewOf(m.spot).sound = s
			w := &m.watch
			w.spot, w.state, w.t0 = m.spot, watchPlaying, now
			clock = m.clock
		}
		var pos time.Duration
		for i := 1; i <= 48; i++ {
			now = now.Add(step)
			s.at = max(0, time.Duration(i)*step-100*time.Millisecond)
			pos = clock(now)
		}
		if e := pos - s.at; e > drift || e < -drift {
			t.Fatalf("%s: the clock is at %v, the sound at %v", name, pos, s.at)
		}
		stuck := s.at
		for range 24 {
			now = now.Add(step)
			next := clock(now)
			if next <= pos {
				t.Fatalf("%s: the picture stood at %v while the sound was stuck", name, pos)
			}
			pos = next
		}
		if pos < stuck+900*time.Millisecond {
			t.Errorf("%s: %v after a second with the sound stuck at %v", name, pos, stuck)
		}
		for range 3 {
			now = now.Add(step)
			s.at += step
			pos = clock(now)
		}
		if e := pos - s.at; e > drift || e < -drift {
			t.Errorf("%s: the clock is at %v once the sound moves again at %v", name, pos, s.at)
		}
	}
}

func TestNetworkPageDoesNotDecode(t *testing.T) {
	m := mediaModel(t, true, 160, 48)
	m.setSection(secNetwork)
	shown := 0
	for i := range m.opts.Site.Spotlights {
		m.selectSpot(i)
		view := strings.Join(plainView(m), "\n")
		c := m.interviewOf(i)
		if c == nil {
			continue
		}
		shown++
		if c.video != nil {
			t.Errorf("spotlight %d decoded by the page", i)
		}
		if !strings.Contains(view, "[ENTER] PLAY") {
			t.Errorf("spotlight %d: no play key", i)
		}
	}
	if shown == 0 {
		t.Fatal("no interviews")
	}
	m.openPlayer()
	if c := m.interviewOf(m.spot); c == nil || c.video == nil || !m.watchOpen {
		t.Error("the interview is not read on opening")
	}
}

func TestDamagedInterview(t *testing.T) {
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	content, err := embedded.LoadSite()
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for _, s := range content.Spotlights {
		if s.Video != "" {
			name := strings.TrimSuffix(path.Base(s.Video), path.Ext(s.Video))
			files[name+".zkv"] = &fstest.MapFile{Data: []byte("not a clip")}
		}
	}
	m := New(Options{Atlas: a, Site: content, Client: market.New(true), Media: files})
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
	m.setSection(secNetwork)
	m.spot = joshSpot(t, m)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.watchOpen || m.interviewOf(m.spot) != nil {
		t.Error("a clip that does not decode opened")
	}
	if strings.Contains(strings.Join(plainView(m), "\n"), "PLAY") {
		t.Error("the play key stays after the clip failed")
	}
}
