package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/audio"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

type montage struct {
	Sound    string        `json:"sound"`
	SoundURL string        `json:"soundURL"`
	Title    string        `json:"title"`
	Items    []montageItem `json:"items"`
}

type montageItem struct {
	Clip  string  `json:"clip"`
	Start float64 `json:"start"`
	In    float64 `json:"in"`
	Len   float64 `json:"len"`
	Loop  bool    `json:"loop"`
	video *video.Video
}

type montagePlayer struct {
	read       bool
	items      []montageItem
	lead       time.Duration
	cols, rows int
	title      string
	mp3        []byte
	soundURL   string
	soundFile  string
	sound      sound
	loading    bool
	soundErr   error
	open       bool
	state      watchState
	at         time.Duration
	t0         time.Time
	base       time.Duration
	synced     bool
	gen        int
}

type montageTickMsg int

type montageSoundMsg struct {
	sound sound
	err   error
}

type montageTrackMsg struct {
	mp3 []byte
	err error
}

func (m *Model) montageRead() {
	p := &m.mont
	if p.read {
		return
	}
	p.read = true
	if m.opts.Media == "" {
		return
	}
	dir := filepath.Join(m.opts.Media, "trailer")
	data, err := os.ReadFile(filepath.Join(dir, "montage.json"))
	if err != nil {
		return
	}
	var mt montage
	if json.Unmarshal(data, &mt) != nil {
		return
	}
	clips := map[string]*video.Video{}
	for _, it := range mt.Items {
		name := filepath.Base(it.Clip)
		v, ok := clips[name]
		if !ok {
			if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				v, _ = video.Read(b)
			}
			clips[name] = v
		}
		if v != nil && it.Len > 0 && it.In >= 0 {
			it.video = v
			p.items = append(p.items, it)
			c, r := v.Size()
			p.cols, p.rows = max(p.cols, c), max(p.rows, r)
		}
	}
	if len(p.items) == 0 {
		return
	}
	sort.Slice(p.items, func(i, j int) bool { return p.items[i].Start < p.items[j].Start })
	p.lead = time.Duration(p.items[0].Start * float64(time.Second))
	p.at = p.lead
	p.title = mt.Title
	if mt.Sound != "" {
		name := filepath.Base(mt.Sound)
		p.soundFile, p.soundURL = name, mt.SoundURL
		p.mp3, _ = os.ReadFile(filepath.Join(dir, name))
		if p.mp3 == nil {
			if path, ok := trackCache(name); ok {
				p.mp3, _ = os.ReadFile(path)
			}
		}
		if p.title == "" {
			p.title = strings.TrimSuffix(name, filepath.Ext(name))
		}
	}
}

func trackCache(name string) (string, bool) {
	dir, err := os.UserCacheDir()
	if err != nil || name == "" {
		return "", false
	}
	return filepath.Join(dir, "zksnarks", name), true
}

func (m *Model) fetchTrack() tea.Cmd {
	m.montageRead()
	p := &m.mont
	if p.mp3 != nil || p.soundURL == "" || !m.online() {
		return nil
	}
	path, ok := trackCache(p.soundFile)
	if !ok {
		return nil
	}
	url := p.soundURL
	return func() tea.Msg {
		data, err := download(url)
		if err == nil {
			if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				err = writeFile(path, data)
			}
		}
		return montageTrackMsg{data, err}
	}
}

func download(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", market.UserAgent)
	res, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err == nil && res.ContentLength >= 0 && int64(len(data)) != res.ContentLength {
		err = fmt.Errorf("%s: %d of %d bytes", url, len(data), res.ContentLength)
	}
	return data, err
}

func writeFile(path string, data []byte) error {
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (m *Model) onMontageTrack(msg montageTrackMsg) tea.Cmd {
	p := &m.mont
	if msg.err != nil || p.mp3 != nil {
		return nil
	}
	p.mp3 = msg.mp3
	if p.state == watchPlaying {
		return m.loadMontageSound()
	}
	return nil
}

func (m *Model) hasMontage() bool {
	m.montageRead()
	return len(m.mont.items) > 0
}

func (p *montagePlayer) frameAt(t time.Duration) (*video.Video, int) {
	if len(p.items) == 0 {
		return nil, 0
	}
	s, it := t.Seconds(), p.items[0]
	pos := it.In
	if s >= it.Start {
		found := false
		for _, x := range p.items {
			if s >= x.Start && s < x.Start+x.Len {
				it, found = x, true
				break
			}
		}
		if !found {
			return nil, 0
		}
		pos = it.In + s - it.Start
	}
	if it.Loop {
		pos = math.Mod(pos, it.video.Duration().Seconds())
	}
	return it.video, it.video.At(time.Duration(pos * float64(time.Second)))
}

func (m *Model) montageLength() time.Duration {
	p := &m.mont
	if l, ok := p.sound.(interface{ Length() time.Duration }); ok {
		return l.Length()
	}
	last := p.items[len(p.items)-1]
	return time.Duration((last.Start + last.Len) * float64(time.Second))
}

func (m *Model) openMontage() tea.Cmd {
	if !m.hasMontage() {
		return nil
	}
	m.mont.open = true
	return m.playMontage()
}

func (m *Model) closeMontage() {
	m.stopMontage()
	m.mont.open = false
}

func (m *Model) playMontage() tea.Cmd {
	p := &m.mont
	if len(p.items) == 0 || p.state == watchPlaying {
		return nil
	}
	if p.state == watchStopped {
		p.at = p.lead
		p.seekSound()
	}
	p.state, p.base, p.t0, p.synced = watchPlaying, p.at, time.Now(), false
	p.gen++
	cmds := []tea.Cmd{m.montageTick()}
	if p.sound != nil {
		p.sound.Play()
	} else {
		cmds = append(cmds, m.loadMontageSound())
	}
	return tea.Batch(cmds...)
}

func (m *Model) loadMontageSound() tea.Cmd {
	p := &m.mont
	if p.mp3 == nil || p.sound != nil || p.loading || p.soundErr != nil {
		return nil
	}
	p.loading = true
	mp3 := p.mp3
	return func() tea.Msg {
		s, err := audio.Load(mp3)
		if err != nil {
			return montageSoundMsg{nil, err}
		}
		return montageSoundMsg{s, nil}
	}
}

func (m *Model) pauseMontage() {
	p := &m.mont
	if p.state != watchPlaying {
		return
	}
	p.at = m.montageClock(time.Now())
	if p.sound != nil {
		p.sound.Pause()
		if p.synced {
			p.at = p.sound.Position()
		}
	}
	p.state = watchPaused
	p.gen++
}

func (m *Model) stopMontage() {
	p := &m.mont
	if p.sound != nil {
		p.sound.Rewind()
	}
	p.state, p.at = watchStopped, p.lead
	p.gen++
}

func (p *montagePlayer) seekSound() {
	if s, ok := p.sound.(interface{ Seek(time.Duration) }); ok {
		s.Seek(p.at)
	}
}

func (m *Model) onMontageSound(msg montageSoundMsg) {
	p := &m.mont
	p.loading, p.sound, p.soundErr = false, msg.sound, msg.err
	p.t0, p.base = time.Now(), p.at
	if p.sound != nil && p.state == watchPlaying {
		p.seekSound()
		p.sound.Play()
	}
}

func (m *Model) onMontageTick(msg montageTickMsg) tea.Cmd {
	p := &m.mont
	if int(msg) != p.gen || p.state != watchPlaying {
		return nil
	}
	pos := m.montageClock(time.Now())
	if pos >= m.montageLength() {
		m.closeMontage()
		return nil
	}
	p.at = pos
	return m.montageTick()
}

func (m *Model) montageTick() tea.Cmd {
	p := &m.mont
	s, next := p.at.Seconds(), 1/24.0
	for _, it := range p.items {
		if s < it.Start {
			next = min(next, it.Start-s)
			break
		}
		if s < it.Start+it.Len {
			fps, local := it.video.FPS(), it.In+s-it.Start
			next = min((math.Floor(local*fps)+1)/fps-local, it.Start+it.Len-s)
			break
		}
	}
	gen := p.gen
	return tea.Tick(max(time.Duration(next*float64(time.Second)), 4*time.Millisecond), func(time.Time) tea.Msg { return montageTickMsg(gen) })
}

func (m *Model) montageClock(now time.Time) time.Duration {
	p := &m.mont
	switch {
	case p.state != watchPlaying:
		return p.at
	case p.loading:
		p.t0 = now
		return p.base
	case p.sound == nil || p.synced && !p.sound.Playing():
		return p.base + now.Sub(p.t0)
	}
	a := p.sound.Position()
	if !p.synced {
		if a <= p.base {
			p.t0 = now
			return p.base
		}
		p.synced, p.t0, p.base = true, now, a
		return a
	}
	pos := p.base + now.Sub(p.t0)
	e := a - pos
	if e > drift || e < -drift {
		p.t0, p.base = now, a
		return a
	}
	p.base += e / 8
	return pos + e/8
}

func (m *Model) montageKey(k string) tea.Cmd {
	switch k {
	case "esc", "backspace":
		m.closeMontage()
	case "p", "space":
		if m.mont.state == watchPlaying {
			m.pauseMontage()
			return nil
		}
		return m.playMontage()
	case "enter":
		return m.playMontage()
	case "s":
		m.stopMontage()
	}
	return nil
}

func (m *Model) montageClick(ms tea.Mouse) tea.Cmd {
	if m.screen.contains(ms.X, ms.Y) {
		return m.montageKey("p")
	}
	return nil
}

func (m *Model) montageKeys() line {
	pairs := []string{"esc", "close", "p", "play"}
	switch m.mont.state {
	case watchPlaying:
		pairs = []string{"esc", "close", "p", "pause", "s", "stop"}
	case watchPaused:
		pairs = []string{"esc", "close", "p", "resume", "s", "stop"}
	}
	return m.hints(pairs...)
}

func (m *Model) captionColour() art.RGB { return mix(m.theme.Dim, m.theme.BodyBg, 0.52) }

func (m *Model) montageScreen(cols, rows int) []line {
	out := make([]line, rows)
	v, frame := m.mont.frameAt(m.mont.at)
	if v == nil {
		return out
	}
	vc, vr := v.Size()
	cw, ch := min(vc, cols), min(vr, rows)
	y0 := (rows - ch) / 2
	lines := v.Frame(frame, m.opts.Glyph)
	for y := range ch {
		l := lines[(vr-ch)/2+y]
		if cw < vc {
			l = cropCells(l, (vc-cw)/2, cw)
		}
		var r row
		r.space((cols - cw) / 2)
		r.cells(l, cw)
		out[y0+y] = r.line()
	}
	return out
}

func (m *Model) montageLines(w, h int) []line {
	p := &m.mont
	dim := fg(m.captionColour())
	under := []line{{}}
	if p.title != "" {
		under = append(under, text(dim, clip(p.title, w)))
	}
	under = append(under, text(dim, mmss(p.at-p.lead)+" / "+mmss(m.montageLength()-p.lead)))
	if min(p.rows, h-len(under)) < montageMinRows {
		under = nil
	}
	cols, rows := min(p.cols, w), min(p.rows, h-len(under))
	top, left := (h-rows-len(under))/2, (w-cols)/2
	out := make([]line, h)
	for i, l := range m.montageScreen(cols, rows) {
		var r row
		r.space(left)
		r.cells(l.s, l.w)
		out[top+i] = r.line()
	}
	for i, l := range under {
		out[top+rows+i] = centreIn(l, w)
	}
	x, y := 2+left, m.headRows()+top
	m.screen = rect{x, y, x + cols, y + rows}
	return out
}

const montageMinRows = 12

func cropCells(s string, from, n int) string {
	var b strings.Builder
	for i, x := 0, 0; i < len(s) && x < from+n; {
		if s[i] == 0x1b {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			b.WriteString(s[i:min(len(s), j+1)])
			i = j + 1
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		if x >= from {
			b.WriteString(s[i : i+size])
		}
		i += size
		x++
	}
	return b.String() + reset
}
