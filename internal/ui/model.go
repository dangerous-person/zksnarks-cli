// Package ui is the Bubble Tea front end: the Zilk Road explorer (gallery, list, card,
// search, filters) and the zksnarks.net sections (luminaries, network).
package ui

import (
	"io/fs"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
	"github.com/dangerous-person/zksnarks-cli/internal/term"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

const (
	minW, minH = 60, 20
	slotW      = art.ThumbW + 2 // a thumbnail with a one-cell margin on each side
	slotGap    = 2
	pitch      = art.ThumbH + 2 // gap line, 13 art lines, label
	wheelLines = 2
	suggRows   = 6
	suggW      = 44
	suggX      = 4
	shadeDist  = 48
	darkShade  = 45
	thumbCache = 2000
)

type mode int

const (
	modeGallery mode = iota
	modeList
)

type focus int

const (
	focusMain focus = iota
	focusQuery
	focusFilters
)

// State is what the app remembers between runs.
type State struct {
	Theme string `json:"theme"`
}

// Options configure a Model.
type Options struct {
	Atlas   *atlas.Atlas
	Site    *site.Site
	Glyph   art.Glyph
	Color   colorprofile.Profile
	NoColor bool
	Client  *market.Client
	State   State
	Query   string
	Media   fs.FS // the interviews, the trailer and the stills; nil for none
}

type history struct {
	loading bool
	entries []market.HistoryEntry
	err     error
}

// Model is the whole app.
type Model struct {
	opts   Options
	tokens []atlas.Token // atlas tokens with live market fields applied
	byID   map[int]int
	ix     *search.Index
	theme  Theme
	quant  *term.Quantiser

	w, h    int
	section int
	mode    mode
	card    bool      // the card of the selected token is open
	box     rect      // where the overlaid card was last drawn, for clicks outside it
	keys    []keyRect // the card's clickable key hints
	focus   focus

	reading    bool
	readTop    int
	spot       int         // network: the picked spotlight
	netTop     int         // the network page's first visible line
	netFollow  bool        // scroll the page to the picked spotlight
	netTargets []netTarget // what clicks on the page act on, as last drawn
	watchOpen  bool        // the interview's window is open
	screen     rect        // where its video is, as last drawn

	lumTop                        int       // luminaries: first visible line
	lumTicks                      int       // steps of the empty slot's animation, which also blink its cursor
	ticking                       bool      // the luminaries' and network's animations are running
	rootDrips, lumDrips, netDrips *dripping // the headings' drops
	shed                          *shedding // the empty slot's figure

	front        *still
	frontRead    bool
	stillTicking bool
	rootWatch    rect

	input     input
	inputWas  string // query text when the search box got focus, for esc
	q         search.Query
	results   []int // token indices
	sel       int   // index into results
	scroll    int   // gallery: first visible line; list: first visible row
	sugg      []search.Suggestion
	suggSel   int
	suggTop   int
	suggFor   string
	suggMoved bool
	facets    *[6]map[string]int
	statusCnt map[string]int
	panel     panel

	thumbs   map[int][]string
	swatches map[int]art.RGB
	artSize  artSize
	skins    map[[2]string]map[art.RGB]bool

	stats     *market.Stats
	rate      *market.Rate
	marketAt  time.Time // last successful snapshot
	feeds     [feedCount]feed
	manual    bool
	blurred   bool
	snapAt    time.Time
	recent    []market.Event
	lastEvent time.Time // events up to this time are applied
	floors    [6]map[string]float64
	floor     float64
	histories map[int]*history
	histFor   int // the token whose full history the card shows instead of its details
	histTop   int // first visible line of that history

	artChanged map[int]bool

	interviews map[int]*interview // by spotlight; nil when there is none
	watch      watch
	mont       montagePlayer

	contactOpen     bool
	contactCopied   bool
	contactShow     rect
	contactQR       []contactCode
	contactClip     *video.Video
	contactClipRead bool
	contactTicking  bool
	contactPlaying  bool
	contactFrame    int
	contactPic      rect
	clearKey        rect
	helpOpen        bool
	helpTop         int

	shimmerTicking bool
	shimmerAt      int
}

// New builds the model. Tokens are copied so live market data can be applied in place.
func New(o Options) *Model {
	m := &Model{
		opts:       o,
		tokens:     append([]atlas.Token(nil), o.Atlas.Tokens...),
		byID:       map[int]int{},
		thumbs:     map[int][]string{},
		swatches:   map[int]art.RGB{},
		histories:  map[int]*history{},
		artChanged: map[int]bool{},
		interviews: map[int]*interview{},
		theme:      themes[0],
		quant:      term.NewQuantiser(o.Color),
		mode:       modeList,
		shimmerAt:  -1,
	}
	m.lastEvent, _ = parseTime(o.Atlas.Meta.SnapshotAt)
	for i, t := range m.tokens {
		m.byID[t.ID] = i
	}
	for _, t := range themes {
		if t.Name == o.State.Theme {
			m.theme = t
		}
	}
	if o.NoColor {
		m.theme = plainTheme
	}
	m.ix = search.NewIndex(m.tokens)
	m.input.set(o.Query)
	m.refresh(false)
	m.computeFloors()
	return m
}

// SavedState returns what to remember for the next run.
func (m *Model) SavedState() State {
	theme := m.theme.Name
	if m.opts.NoColor {
		theme = m.opts.State.Theme
	}
	return State{Theme: theme}
}

func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	switch m.section {
	case secRoot:
		cmds = append(cmds, m.startPageTick(), m.startStill())
	case secLuminaries, secNetwork:
		cmds = append(cmds, m.startPageTick())
	}
	cmds = append(cmds, m.startShimmer(), m.emptyTick())
	cmds = append(cmds, m.startFeeds())
	return tea.Batch(cmds...)
}

// refresh re-runs the query. The selection stays on the same token when it is still in the
// results.
func (m *Model) refresh(keep bool) {
	selID := -1
	if keep && m.sel < len(m.results) {
		selID = m.tokens[m.results[m.sel]].ID
	}
	m.q = search.Parse(m.input.text, m.ix.Cat)
	m.results = m.ix.Run(m.q, m.tokens)
	m.facets, m.statusCnt = nil, nil
	m.sel = 0
	if selID >= 0 {
		for i, r := range m.results {
			if m.tokens[r].ID == selID {
				m.sel = i
				break
			}
		}
	} else {
		m.scroll = 0
	}
	if m.sel >= len(m.results) {
		m.sel = max(0, len(m.results)-1)
	}
	m.updateSuggestions()
	m.ensureVisible()
}

func (m *Model) updateSuggestions() {
	keep, top, moved := "", m.suggTop, m.suggMoved
	if m.suggFor == m.input.text && m.suggSel < len(m.sugg) {
		keep = m.sugg[m.suggSel].Text()
	}
	m.sugg, m.suggSel, m.suggTop, m.suggFor, m.suggMoved = nil, 0, 0, m.input.text, false
	if m.focus == focusQuery && m.input.pos == len(m.input.r) {
		m.sugg = m.ix.Suggest(m.input.text, m.results, m.tokens)
	}
	for i, s := range m.sugg {
		if keep != "" && s.Text() == keep {
			m.suggTop = top
			m.pickSuggestion(i)
			m.suggMoved = moved
			break
		}
	}
}

func (m *Model) pickSuggestion(i int) {
	m.suggSel = max(0, min(i, len(m.sugg)-1))
	m.suggTop = max(m.suggSel-suggRows+1, min(m.suggTop, m.suggSel))
	m.suggMoved = len(m.sugg) > 0
}

func (m *Model) scrollSuggestions(d int) {
	m.suggTop = max(0, min(m.suggTop+d, len(m.sugg)-suggRows))
	m.suggSel = max(m.suggTop, min(m.suggSel, m.suggTop+suggRows-1))
	m.suggMoved = true
}

func (m *Model) enterCompletes() bool {
	if len(m.sugg) == 0 {
		return false
	}
	s := m.sugg[m.suggSel]
	parts := strings.Fields(m.input.text)
	return m.suggMoved || len(parts) == 0 || s.Term != "" || s.Jump != 0 || strings.Contains(parts[len(parts)-1], ":")
}

func (m *Model) complete(s search.Suggestion) tea.Cmd {
	if s.Jump != 0 {
		return m.jump(s.Jump)
	}
	m.input.set(search.Complete(m.input.text, s))
	m.refresh(true)
	return nil
}

func (m *Model) selected() (*atlas.Token, bool) {
	if m.sel < 0 || m.sel >= len(m.results) {
		return nil, false
	}
	return &m.tokens[m.results[m.sel]], true
}

func (m *Model) thumb(i int) []string {
	if t, ok := m.thumbs[i]; ok {
		return t
	}
	if len(m.thumbs) >= thumbCache {
		clear(m.thumbs)
	}
	t := art.Thumb(m.artOf(i), m.opts.Glyph)
	m.thumbs[i] = t
	return t
}

func (m *Model) cardArt(i int) []string { return art.Card(m.artOf(i)) }

func (m *Model) previewArt(i int) []string { return art.Card(m.tokens[i].Art) }

func (m *Model) previewThumb(i int) []string {
	if !m.tokens[i].Purged {
		return m.thumb(i)
	}
	return art.Thumb(m.tokens[i].Art, m.opts.Glyph)
}

// artOf is the art to draw for a token: greyscale once it is purged, as on the site.
func (m *Model) artOf(i int) art.Art {
	if m.tokens[i].Purged {
		return art.Grey(m.tokens[i].Art)
	}
	return m.tokens[i].Art
}

// Update handles a message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	return m, tea.Batch(cmd, m.startTicks())
}

func (m *Model) startTicks() tea.Cmd {
	if m.blurred {
		return nil
	}
	cmds := []tea.Cmd{m.startShimmer(), m.emptyTick()}
	switch m.section {
	case secRoot:
		cmds = append(cmds, m.startPageTick(), m.startStill())
	case secLuminaries, secNetwork:
		cmds = append(cmds, m.startPageTick())
	}
	return tea.Batch(cmds...)
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.ensureVisible()
		m.panel.clamp(m)
		return m, m.fitPlayer()
	case tea.KeyPressMsg:
		return m, tea.Batch(m.focusIn(), m.key(msg), m.emptyTick())
	case tea.PasteMsg:
		if m.focus == focusQuery && m.input.paste(msg.Content) {
			m.refresh(true)
		}
		return m, tea.Batch(m.focusIn(), m.emptyTick())
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())
		return m, nil
	case tea.MouseClickMsg:
		return m, tea.Batch(m.click(msg.Mouse()), m.emptyTick())
	case pageTickMsg:
		return m, m.onPageTick()
	case watchTickMsg:
		return m, m.onWatchTick(msg)
	case stillTickMsg:
		return m, m.onStillTick()
	case shimmerTickMsg:
		return m, m.onShimmerTick()
	case contactTickMsg:
		return m, m.onContactTick()
	case montageTickMsg:
		return m, m.onMontageTick(msg)
	case montageSoundMsg:
		m.onMontageSound(msg)
		return m, nil
	case soundMsg:
		m.onSound(msg)
		return m, nil
	}
	return m, m.marketMsg(msg)
}

// keyName names a key. Printable ASCII keys use their text; for anything else Keystroke
// prefers the PC-101 base key, which terminals with the kitty keyboard protocol report, so
// letter shortcuts keep working with other keyboard layouts there.
func keyName(msg tea.KeyPressMsg) string {
	if msg.Text == " " {
		return "space"
	}
	if msg.Text != "" && msg.Mod&^tea.ModShift == 0 {
		if r := []rune(msg.Text); len(r) == 1 && r[0] < 128 {
			return msg.Text
		}
	}
	return msg.Keystroke()
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := keyName(msg)
	if k == "ctrl+c" {
		return tea.Quit
	}
	if m.focus == focusQuery {
		return m.queryKey(msg, k)
	}
	if m.focus == focusFilters {
		return m.panelKey(k)
	}
	if m.helpShown() {
		return m.helpKey(k)
	}
	switch k {
	case "q":
		return tea.Quit
	case "t":
		if !m.opts.NoColor {
			m.cycleTheme()
		}
		return nil
	case "r", "R":
		if cmd, ok := m.retryHistory(); ok {
			return cmd
		}
		return m.retryMarket()
	case "0", "1", "2", "3", "4", "5":
		return m.setSection(int(k[0] - '0'))
	case "/":
		m.setSection(secZilkroad)
		m.card = false
		m.focusQuery()
		return nil
	}
	switch m.section {
	case secRoot:
		return m.rootKey(k)
	case secManifesto:
		return m.manifestoKey(k)
	case secLuminaries:
		m.luminaryKey(k)
		return nil
	case secNetwork:
		return m.networkKey(k)
	case secAbout:
		return m.contactKey(k)
	}
	if m.card {
		return m.cardKey(k)
	}
	switch k {
	case "?":
		m.helpOpen, m.helpTop = true, 0
	case "f":
		m.openPanel()
	case "z", "a", "H", "shift+h":
		return m.tokenKey(k)
	case "h":
		if m.mode == modeList {
			return m.tokenKey(k)
		}
		m.move(k)
	case "+", "=", "-":
		if m.mode == modeList {
			m.resizeArt(k != "-")
		}
	case "v":
		if m.mode == modeGallery {
			m.mode = modeList
		} else {
			m.mode = modeGallery
		}
		m.scroll = 0
		m.ensureVisible()
	case "enter":
		if len(m.results) > 0 {
			m.card, m.histFor = true, 0
		}
	case "esc":
		if m.input.text != "" {
			m.input.set("")
			m.refresh(true)
		}
	default:
		m.move(k)
	}
	return nil
}

func (m *Model) tokenKey(k string) tea.Cmd {
	t, ok := m.selected()
	if !ok {
		return nil
	}
	switch k {
	case "z":
		return openURL(zilkroadURL(t.ID))
	case "a":
		if ap, ok := m.burntFor(t); ok {
			return m.jump(ap.Token)
		}
		return nil
	}
	m.card, m.histFor = true, 0
	return m.toggleHistory()
}

func (m *Model) setSection(s int) tea.Cmd {
	if s < 0 || s >= len(sectionNames) || s == m.section {
		return nil
	}
	m.section = s
	m.focus = focusMain
	m.sugg = nil
	m.closePlayer()
	m.closeMontage()
	m.contactOpen, m.contactCopied = false, false
	m.helpOpen = false
	shimmer := m.startShimmer()
	switch {
	case s == secZilkroad:
		m.ensureVisible()
	case s == secRoot:
		return tea.Batch(shimmer, m.startPageTick(), m.startStill())
	case s == secLuminaries || s == secNetwork:
		return m.startPageTick()
	}
	return shimmer
}

func (m *Model) leaveQuery() {
	m.focus = focusMain
	m.sugg = nil
	m.input.set(strings.TrimSpace(m.input.text))
}

func (m *Model) focusQuery() {
	m.focus = focusQuery
	m.inputWas = m.input.text
	m.input.end()
	m.updateSuggestions()
}

func (m *Model) cycleTheme() {
	for i, t := range themes {
		if t.Name == m.theme.Name {
			m.theme = themes[(i+1)%len(themes)]
			return
		}
	}
}

func (m *Model) queryKey(msg tea.KeyPressMsg, k string) tea.Cmd {
	switch k {
	case "?":
		m.leaveQuery()
		m.helpOpen, m.helpTop = true, 0
		return nil
	case "enter":
		if m.enterCompletes() {
			return m.complete(m.sugg[m.suggSel])
		}
		m.leaveQuery()
		return nil
	case "esc":
		m.focus = focusMain
		m.input.set(m.inputWas)
		m.refresh(true)
		return nil
	case "tab":
		if len(m.sugg) > 0 {
			return m.complete(m.sugg[m.suggSel])
		}
		return nil
	case "down", "ctrl+n":
		m.pickSuggestion(m.suggSel + 1)
		return nil
	case "up", "ctrl+p":
		m.pickSuggestion(m.suggSel - 1)
		return nil
	case "pgdown":
		m.pickSuggestion(m.suggSel + suggRows)
		return nil
	case "pgup":
		m.pickSuggestion(m.suggSel - suggRows)
		return nil
	}
	if m.input.key(msg) {
		m.refresh(true)
	}
	return nil
}

// jump opens the card of a token number, showing it in the results even if it is purged.
func (m *Model) jump(id int) tea.Cmd {
	if _, ok := m.byID[id]; !ok {
		return nil
	}
	m.focus = focusMain
	m.section = secZilkroad
	m.input.set("#" + itoa(id))
	m.refresh(false)
	m.card, m.histFor = len(m.results) > 0, 0
	m.ensureVisible()
	return nil
}

// move handles navigation keys in the gallery and the list.
func (m *Model) move(k string) {
	n := len(m.results)
	if n == 0 {
		return
	}
	cols, page := 1, max(1, m.bodyH()-1)
	if m.mode == modeGallery {
		cols = m.cols()
		page = max(1, m.bodyH()/pitch) * cols
	}
	switch k {
	case "left", "h":
		if m.mode == modeGallery {
			m.sel--
		}
	case "right", "l":
		if m.mode == modeGallery {
			m.sel++
		}
	case "up", "k":
		m.sel -= cols
	case "down", "j":
		if m.sel+cols < n {
			m.sel += cols
		} else if m.sel/cols < (n-1)/cols {
			m.sel = n - 1 // onto the shorter last row
		}
	case "pgup", "ctrl+b", "ctrl+u":
		m.sel -= page
	case "pgdown", "ctrl+f", "ctrl+d", "space":
		m.sel += page
	case "home", "g":
		m.sel = 0
	case "end", "G":
		m.sel = n - 1
	default:
		return
	}
	m.sel = max(0, min(m.sel, n-1))
	m.ensureVisible()
}

func (m *Model) innerW() int { return max(1, m.w-4) }

// resultsW is the width left for the results next to the filters panel.
func (m *Model) resultsW() int {
	w := m.innerW()
	if m.panel.open {
		w -= panelW + 2
	}
	return max(1, w)
}

func (m *Model) cols() int { return max(1, (m.resultsW()+slotGap)/(slotW+slotGap)) }

func (m *Model) contentH() int {
	if m.mode == modeList {
		return len(m.results)
	}
	rows := (len(m.results) + m.cols() - 1) / m.cols()
	return rows*pitch + 1
}

func (m *Model) viewH() int {
	if m.mode == modeList {
		return max(1, m.bodyH()-1) // one line for the column header
	}
	return m.bodyH()
}

func (m *Model) clampScroll() {
	m.scroll = max(0, min(m.scroll, m.contentH()-m.viewH()))
}

// ensureVisible scrolls so the whole selected slot (with its corner marks) is on screen.
func (m *Model) ensureVisible() {
	if m.w == 0 {
		return
	}
	if m.mode == modeList {
		if m.sel < m.scroll {
			m.scroll = m.sel
		}
		if m.sel >= m.scroll+m.viewH() {
			m.scroll = m.sel - m.viewH() + 1
		}
		m.clampScroll()
		return
	}
	r := m.sel / m.cols()
	top, bottom := r*pitch, r*pitch+pitch
	if bottom >= m.scroll+m.viewH() {
		m.scroll = bottom - m.viewH() + 1
	}
	if top < m.scroll {
		m.scroll = top
	}
	m.clampScroll()
}

// keepSelectionInView moves the selection after a wheel scroll so it stays on screen.
func (m *Model) keepSelectionInView() {
	n := len(m.results)
	if n == 0 {
		return
	}
	if m.mode == modeList {
		m.sel = max(m.scroll, min(m.sel, m.scroll+m.viewH()-1, n-1))
		return
	}
	cols := m.cols()
	r, c := m.sel/cols, m.sel%cols
	first := (m.scroll + pitch - 1) / pitch        // first row whose top gap line is visible
	last := (m.scroll+m.viewH()-pitch-1)/pitch + 0 // last row whose bottom gap line is visible
	if last < first {
		last = first
	}
	r = max(first, min(r, last))
	m.sel = max(0, min(r*cols+c, n-1))
}

func (m *Model) wheel(ms tea.Mouse) {
	if m.helpShown() {
		if ms.Button == tea.MouseWheelUp {
			m.helpTop--
		} else if ms.Button == tea.MouseWheelDown {
			m.helpTop++
		}
		return
	}
	switch m.section {
	case secManifesto:
		if m.reading {
			if ms.Button == tea.MouseWheelUp {
				m.readTop -= wheelLines
			} else if ms.Button == tea.MouseWheelDown {
				m.readTop += wheelLines
			}
		}
		return
	case secRoot:
		return
	case secLuminaries:
		switch ms.Button {
		case tea.MouseWheelUp:
			m.lumTop -= wheelLines
		case tea.MouseWheelDown:
			m.lumTop += wheelLines
		}
		return
	case secNetwork:
		if !m.watchOpen {
			switch ms.Button {
			case tea.MouseWheelUp:
				m.netTop -= wheelLines
			case tea.MouseWheelDown:
				m.netTop += wheelLines
			}
		}
		return
	}
	if m.inSuggestions(ms.X, ms.Y) {
		switch ms.Button {
		case tea.MouseWheelUp:
			m.scrollSuggestions(-1)
		case tea.MouseWheelDown:
			m.scrollSuggestions(1)
		}
		return
	}
	if m.panel.open && ms.X < panelW+2 && !m.card {
		m.panel.wheel(m, ms.Button)
		return
	}
	if m.card {
		if t, ok := m.selected(); ok && m.histFor == t.ID {
			switch ms.Button {
			case tea.MouseWheelUp:
				m.histTop -= wheelLines
			case tea.MouseWheelDown:
				m.histTop += wheelLines
			}
		}
		return
	}
	step := wheelLines
	if m.mode == modeList {
		step = 1
	}
	switch ms.Button {
	case tea.MouseWheelUp:
		m.scroll -= step
	case tea.MouseWheelDown:
		m.scroll += step
	default:
		return
	}
	m.clampScroll()
	m.keepSelectionInView()
}

func (m *Model) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft {
		return nil
	}
	if ms.Y == 1 {
		items := m.navLayout()
		for _, it := range items {
			if ms.X >= it.x0 && ms.X < it.x1 {
				if it.sec == m.section && m.focus == focusQuery {
					m.leaveQuery()
				}
				return m.setSection(it.sec)
			}
		}
		return nil
	}
	switch m.section {
	case secRoot:
		return m.rootClick(ms)
	case secNetwork:
		return m.networkClick(ms)
	case secAbout:
		return m.contactClick(ms)
	}
	if m.section != secZilkroad {
		return nil
	}
	if m.helpShown() {
		if !m.box.contains(ms.X, ms.Y) {
			m.helpOpen = false
		}
		return nil
	}
	if m.overlay() {
		if !m.box.contains(ms.X, ms.Y) {
			m.card = false // a click outside the card closes it
			return nil
		}
		return m.clickKey(ms)
	}
	if m.inSuggestions(ms.X, ms.Y) {
		i := m.suggTop + ms.Y - m.headRows() - 1
		if x0 := m.suggX(); ms.X > x0 && ms.X < x0+suggW-1 && i >= m.suggTop && i < min(len(m.sugg), m.suggTop+suggRows) {
			return m.complete(m.sugg[i])
		}
		return nil
	}
	if ms.Y == 3 && !m.card {
		if m.clearKey.contains(ms.X, ms.Y) {
			m.input.set("")
			m.refresh(true)
			return nil
		}
		m.focusQuery()
		return nil
	}
	if m.focus == focusQuery {
		m.leaveQuery()
	}
	if m.card {
		return m.clickKey(ms)
	}
	if m.mode == modeList {
		for _, k := range m.keys { // the preview's key hints
			if k.contains(ms.X, ms.Y) {
				if k.key == "+" || k.key == "-" {
					m.resizeArt(k.key == "+")
					return nil
				}
				return m.tokenKey(k.key)
			}
		}
	}
	top := m.headRows()
	y, x := ms.Y-top, ms.X-2
	if y < 0 || y >= m.viewH()+boolInt(m.mode == modeList) || x < 0 {
		return nil
	}
	if m.panel.open {
		if x < panelW {
			m.panel.click(m, y)
			return nil
		}
		x -= panelW + 2
	}
	idx := -1
	if m.mode == modeList {
		if _, tableW, _, _ := m.listLayout(m.resultsW(), m.bodyH()); y == 0 || x >= tableW {
			return nil // the header, or the preview next to the table
		}
		idx = m.scroll + y - 1
	} else {
		cols := m.cols()
		gridW := cols*slotW + (cols-1)*slotGap
		x -= (m.resultsW() - gridW) / 2
		ln := m.scroll + y
		r, k := ln/pitch, ln%pitch
		if x < 0 || x%(slotW+slotGap) >= slotW || k == 0 || x/(slotW+slotGap) >= cols {
			return nil
		}
		idx = r*cols + x/(slotW+slotGap)
	}
	if idx < 0 || idx >= len(m.results) {
		return nil
	}
	if idx == m.sel {
		m.card, m.histFor = true, 0
		m.ensureVisible()
		return nil
	}
	m.sel = idx
	m.focus = focusMain
	m.ensureVisible()
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
