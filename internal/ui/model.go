// Package ui is the Bubble Tea front end: the Zilk Road explorer (gallery, card, search,
// filters) and the zksnarks.net sections (luminaries, network).
package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

const (
	minW, minH = 60, 20
	slotW      = art.ThumbW + 2 // a thumbnail with a one-cell margin on each side
	slotGap    = 2
	pitch      = art.ThumbH + 2 // gap line, 13 art lines, label
	wheelLines = 2
)

type focus int

const (
	focusMain focus = iota
	focusQuery
	focusFilters
)

// State is what the app remembers between runs.
type State struct {
	Query   string `json:"query"`
	Theme   string `json:"theme"`
	Section string `json:"section"`
}

// Options configure a Model.
type Options struct {
	Atlas  *atlas.Atlas
	Site   *site.Site
	Glyph  art.Glyph
	Client *market.Client
	State  State
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

	w, h    int
	section int
	card    bool      // the card of the selected token is open
	box     rect      // where the overlaid card was last drawn, for clicks outside it
	keys    []keyRect // the card's clickable key hints
	focus   focus
	help    bool

	spot       int         // network: the picked spotlight
	netTop     int         // the network page's first visible line
	netFollow  bool        // scroll the page to the picked spotlight
	netTargets []netTarget // what clicks on the page act on, as last drawn

	lumTop   int       // luminaries: first visible line
	lumTicks int       // steps of the empty slot's animation, which also blink its cursor
	ticking  bool      // they are running
	shed     *shedding // the empty slot's figure

	input     input
	inputWas  string // query text when the search box got focus, for esc
	q         search.Query
	results   []int // token indices
	sel       int   // index into results
	scroll    int   // first visible line of the gallery
	sugg      []search.Suggestion
	suggSel   int // -1: none highlighted
	facets    *[6]map[string]int
	statusCnt map[string]int
	panel     panel

	thumbs map[int][]string
	cards  map[int][]string

	stats      *market.Stats
	rate       *market.Rate
	marketAt   time.Time // last successful snapshot
	marketErr  error
	lastEvent  string // time of the newest applied event
	floors     [6]map[string]float64
	floor      float64
	histories  map[int]*history
	histFor    int // the token whose full history the card shows instead of its details
	histTop    int // first visible line of that history
	artChanged map[int]bool
}

// New builds the model. Tokens are copied so live market data can be applied in place.
func New(o Options) *Model {
	m := &Model{
		opts:       o,
		tokens:     append([]atlas.Token(nil), o.Atlas.Tokens...),
		byID:       map[int]int{},
		thumbs:     map[int][]string{},
		cards:      map[int][]string{},
		histories:  map[int]*history{},
		artChanged: map[int]bool{},
		theme:      themes[0],
		suggSel:    -1,
		lastEvent:  o.Atlas.Meta.SnapshotAt,
	}
	for i, t := range m.tokens {
		m.byID[t.ID] = i
	}
	for _, t := range themes {
		if t.Name == o.State.Theme {
			m.theme = t
		}
	}
	for i, name := range sectionNames {
		if name == o.State.Section {
			m.section = i
		}
	}
	m.ix = search.NewIndex(m.tokens)
	m.input.set(o.State.Query)
	m.refresh(false)
	m.computeFloors()
	return m
}

// SavedState returns what to remember for the next run.
func (m *Model) SavedState() State {
	return State{Query: m.input.text, Theme: m.theme.Name, Section: sectionNames[m.section]}
}

// Init starts the market feed, and the luminaries' animation when the app opens on them.
func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.section == secLuminaries {
		cmds = append(cmds, m.startLuminaryTick())
	}
	if m.online() {
		cmds = append(cmds, m.fetchSnapshot(), m.fetchStats(), m.fetchRate(), tickEvents(), tickSnapshot(), tickRate())
	}
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
	m.sugg, m.suggSel = nil, -1
	if m.focus == focusQuery && m.input.pos == len(m.input.r) {
		m.sugg = m.ix.Suggest(m.input.text, m.results, m.tokens, 6)
	}
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
	t := art.Thumb(m.artOf(i), m.opts.Glyph)
	m.thumbs[i] = t
	return t
}

func (m *Model) cardArt(i int) []string {
	if c, ok := m.cards[i]; ok {
		return c
	}
	c := art.Card(m.artOf(i))
	m.cards[i] = c
	return c
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.ensureVisible()
		m.panel.clamp(m)
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())
		return m, nil
	case tea.MouseClickMsg:
		return m, m.click(msg.Mouse())
	case luminaryTickMsg:
		return m, m.onLuminaryTick()
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
	if m.help {
		m.help = false
		return nil
	}
	if m.focus == focusFilters {
		return m.panelKey(msg, k)
	}
	switch k {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
		return nil
	case "t":
		m.cycleTheme()
		return nil
	case "r":
		return m.refreshMarket()
	case "1", "2", "3":
		return m.setSection(int(k[0] - '1'))
	case "/":
		m.setSection(secZilkroad)
		m.card = false
		m.focusQuery()
		return nil
	}
	switch m.section {
	case secLuminaries:
		m.luminaryKey(k)
		return nil
	case secNetwork:
		return m.networkKey(k)
	}
	if m.card {
		return m.cardKey(k)
	}
	switch k {
	case "f":
		m.openPanel()
	case "z", "H", "shift+h":
		return m.tokenKey(k)
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

// tokenKey opens the selected token on zilkroad (z) or its history on the card (H), from
// the results.
func (m *Model) tokenKey(k string) tea.Cmd {
	t, ok := m.selected()
	if !ok {
		return nil
	}
	if k == "z" {
		return openURL(zilkroadURL(t.ID))
	}
	m.card, m.histFor = true, 0
	return m.toggleHistory()
}

// setSection switches sections; the luminaries page starts its animation.
func (m *Model) setSection(s int) tea.Cmd {
	if s < 0 || s >= len(sectionNames) || s == m.section {
		return nil
	}
	m.section = s
	m.help = false
	if s == secZilkroad {
		m.ensureVisible()
		return nil
	}
	if s == secLuminaries {
		return m.startLuminaryTick()
	}
	return nil
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
	case "enter":
		if m.suggSel >= 0 && m.suggSel < len(m.sugg) && m.sugg[m.suggSel].Jump != 0 {
			return m.jump(m.sugg[m.suggSel].Jump)
		}
		m.focus = focusMain
		m.sugg = nil
		m.input.set(strings.TrimSpace(m.input.text))
		return nil
	case "esc":
		m.focus = focusMain
		m.input.set(m.inputWas)
		m.refresh(true)
		return nil
	case "tab":
		if len(m.sugg) > 0 {
			s := m.sugg[max(0, m.suggSel)]
			if s.Jump != 0 {
				return m.jump(s.Jump)
			}
			m.input.set(search.Complete(m.input.text, s))
			m.refresh(true)
		}
		return nil
	case "down", "ctrl+n":
		if len(m.sugg) > 0 {
			m.suggSel = min(m.suggSel+1, len(m.sugg)-1)
		}
		return nil
	case "up", "ctrl+p":
		if m.suggSel >= 0 {
			m.suggSel--
		}
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

// move handles navigation keys in the gallery.
func (m *Model) move(k string) {
	n := len(m.results)
	if n == 0 {
		return
	}
	cols := m.cols()
	page := max(1, m.bodyH()/pitch) * cols
	switch k {
	case "left", "h":
		m.sel--
	case "right", "l":
		m.sel++
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
	rows := (len(m.results) + m.cols() - 1) / m.cols()
	return rows*pitch + 1
}

func (m *Model) viewH() int { return m.bodyH() }

func (m *Model) clampScroll() {
	m.scroll = max(0, min(m.scroll, m.contentH()-m.viewH()))
}

// ensureVisible scrolls so the whole selected slot (with its corner marks) is on screen.
func (m *Model) ensureVisible() {
	if m.w == 0 {
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
	if m.help {
		return
	}
	switch m.section {
	case secLuminaries:
		switch ms.Button {
		case tea.MouseWheelUp:
			m.lumTop -= wheelLines
		case tea.MouseWheelDown:
			m.lumTop += wheelLines
		}
		return
	case secNetwork:
		switch ms.Button {
		case tea.MouseWheelUp:
			m.netTop -= wheelLines
		case tea.MouseWheelDown:
			m.netTop += wheelLines
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
	switch ms.Button {
	case tea.MouseWheelUp:
		m.scroll -= wheelLines
	case tea.MouseWheelDown:
		m.scroll += wheelLines
	default:
		return
	}
	m.clampScroll()
	m.keepSelectionInView()
}

func (m *Model) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft || m.help {
		return nil
	}
	if ms.Y == 1 {
		items := m.navLayout()
		for _, it := range items {
			if ms.X >= it.x0 && ms.X < it.x1 {
				return m.setSection(it.sec)
			}
		}
		return nil
	}
	if m.section == secNetwork {
		return m.networkClick(ms)
	}
	if m.section != secZilkroad {
		return nil
	}
	if m.overlay() {
		if !m.box.contains(ms.X, ms.Y) {
			m.card = false // a click outside the card closes it
			return nil
		}
		return m.clickKey(ms)
	}
	if ms.Y == 3 && !m.card {
		m.focusQuery()
		return nil
	}
	if m.card {
		return m.clickKey(ms)
	}
	top := m.headRows() + m.suggestionLines()
	y, x := ms.Y-top, ms.X-2
	if y < 0 || y >= m.viewH() || x < 0 {
		return nil
	}
	if m.panel.open {
		if x < panelW {
			m.panel.click(m, y)
			return nil
		}
		x -= panelW + 2
	}
	cols := m.cols()
	gridW := cols*slotW + (cols-1)*slotGap
	x -= (m.resultsW() - gridW) / 2
	ln := m.scroll + y
	r, k := ln/pitch, ln%pitch
	if x < 0 || x%(slotW+slotGap) >= slotW || k == 0 || x/(slotW+slotGap) >= cols {
		return nil
	}
	idx := r*cols + x/(slotW+slotGap)
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
