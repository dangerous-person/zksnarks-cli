package ui

import (
	"context"
	"errors"
	"math/rand/v2"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/search"
)

// The feed polls collection-wide endpoints only: the snapshot of all tokens and the stats
// every 5 minutes, the ZEC/USD rate every minute like the site, and the small events list
// every 30 seconds in between.
const (
	snapshotEvery = 5 * time.Minute
	rateEvery     = time.Minute
	eventsEvery   = 30 * time.Second
	timeout       = 30 * time.Second
	retryFirst    = time.Minute
	retryMax      = 10 * time.Minute
)

type feedKind int

const (
	feedSnapshot feedKind = iota
	feedStats
	feedRate
	feedEvents
	feedCount
)

var feedEvery = [feedCount]time.Duration{snapshotEvery, snapshotEvery, rateEvery, eventsEvery}

type feed struct {
	seq, gen int
	inflight bool
	applied  bool
	failed   bool
	fails    int
	etag     string
	due      time.Time
}

type (
	feedMsg struct {
		kind feedKind
		seq  int
		val  any
		etag string
		err  error
	}
	feedTickMsg struct {
		kind feedKind
		gen  int
	}
	historyMsg struct {
		id  int
		e   []market.HistoryEntry
		err error
	}
)

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func (m *Model) fetch(k feedKind) tea.Cmd {
	f := &m.feeds[k]
	f.seq++
	f.inflight = true
	c, seq, etag := m.opts.Client, f.seq, f.etag
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		msg := feedMsg{kind: k, seq: seq}
		switch k {
		case feedSnapshot:
			msg.val, msg.etag, msg.err = c.Snapshot(cx, etag)
		case feedStats:
			msg.val, msg.etag, msg.err = c.Stats(cx, etag)
		case feedRate:
			msg.val, msg.etag, msg.err = c.Rate(cx, etag)
		case feedEvents:
			msg.val, msg.etag, msg.err = c.Events(cx, etag)
		}
		return msg
	}
}

func (m *Model) schedule(k feedKind) tea.Cmd {
	f := &m.feeds[k]
	d := feedEvery[k]
	if f.failed {
		d = backoff(f.fails)
	}
	d = jitter(d)
	f.due = time.Now().Add(d)
	f.gen++
	gen := f.gen
	return tea.Tick(d, func(time.Time) tea.Msg { return feedTickMsg{k, gen} })
}

func backoff(fails int) time.Duration {
	d := retryFirst
	for i := 1; i < fails && d < retryMax; i++ {
		d *= 2
	}
	return min(d, retryMax)
}

func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.9 + 0.2*rand.Float64()))
}

func (m *Model) online() bool { return m.opts.Client != nil && !m.opts.Client.Offline }

func (m *Model) startFeeds() tea.Cmd {
	if !m.online() {
		return nil
	}
	return tea.Batch(m.fetch(feedSnapshot), m.fetch(feedStats), m.fetch(feedRate), m.schedule(feedEvents))
}

func (m *Model) retryMarket() tea.Cmd {
	if !m.online() || m.fetching() {
		return nil
	}
	m.manual = true
	var cmds []tea.Cmd
	for k := range feedCount {
		m.feeds[k].fails = 0
		cmds = append(cmds, m.fetch(k))
	}
	return tea.Batch(cmds...)
}

func (m *Model) focusIn() tea.Cmd {
	if !m.blurred {
		return nil
	}
	m.blurred = false
	if !m.online() {
		return nil
	}
	now := time.Now()
	var cmds []tea.Cmd
	for k := range feedCount {
		if f := &m.feeds[k]; !f.inflight && !now.Before(f.due) {
			cmds = append(cmds, m.fetch(k))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) fetching() bool {
	for _, f := range m.feeds {
		if f.inflight {
			return true
		}
	}
	return false
}

func (m *Model) failing() bool {
	for _, f := range m.feeds {
		if f.failed {
			return true
		}
	}
	return false
}

func (m *Model) live() bool { return m.feeds[feedSnapshot].applied && m.feeds[feedStats].applied }

func (m *Model) refreshing() bool {
	if m.manual {
		return true
	}
	s, st := &m.feeds[feedSnapshot], &m.feeds[feedStats]
	return !m.live() && (s.inflight || st.inflight) && !s.failed && !st.failed
}

func (m *Model) loadHistory() tea.Cmd {
	t, ok := m.selected()
	if !ok {
		return nil
	}
	if h := m.histories[t.ID]; h != nil && (h.loading || h.err == nil) {
		return nil
	}
	if !m.online() {
		m.histories[t.ID] = &history{err: market.ErrOffline}
		return nil
	}
	id := t.ID
	m.histories[id] = &history{loading: true}
	c := m.opts.Client
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		e, err := c.History(cx, id)
		return historyMsg{id, e, err}
	}
}

func (m *Model) marketMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case feedMsg:
		return m.onFeed(msg)
	case feedTickMsg:
		if f := &m.feeds[msg.kind]; msg.gen != f.gen || f.inflight || m.blurred || !m.online() {
			return nil
		}
		return m.fetch(msg.kind)
	case tea.BlurMsg:
		m.blurred = true
	case tea.FocusMsg:
		return m.focusIn()
	case historyMsg:
		m.histories[msg.id] = &history{entries: msg.e, err: msg.err}
	}
	return nil
}

func (m *Model) onFeed(msg feedMsg) tea.Cmd {
	f := &m.feeds[msg.kind]
	if msg.seq != f.seq {
		return nil
	}
	f.inflight = false
	switch {
	case msg.err == nil:
		if m.apply(msg.val) {
			f.etag, f.applied = msg.etag, true
		}
		f.failed, f.fails = false, 0
	case errors.Is(msg.err, market.ErrNotModified):
		if msg.kind == feedSnapshot {
			m.marketAt = time.Now()
		}
		f.failed, f.fails = false, 0
	default:
		f.failed = true
		f.fails++
	}
	if !m.fetching() {
		m.manual = false
	}
	return m.schedule(msg.kind)
}

func (m *Model) apply(v any) bool {
	switch v := v.(type) {
	case *market.Snapshot:
		return m.applySnapshot(v)
	case *market.Stats:
		m.stats = v
	case *market.Rate:
		m.rate = v
	case []market.Event:
		m.recent = v
		if m.applyEvents(v) {
			m.marketChanged()
		}
	}
	return true
}

func (m *Model) applySnapshot(s *market.Snapshot) bool {
	at, timed := parseTime(s.At)
	if timed && at.After(time.Now().Add(5*time.Minute)) {
		timed = false
	}
	if timed && at.Before(m.snapAt) {
		return false
	}
	for _, lt := range s.Tokens {
		i, ok := m.byID[lt.ID]
		if !ok {
			continue
		}
		t := &m.tokens[i]
		if t.Purged != lt.Purged {
			delete(m.thumbs, i) // purged art is drawn in grey
		}
		t.Listed, t.Ask, t.Bid, t.Last, t.Purged, t.Auction = lt.Listed, string(lt.Ask), string(lt.Bid), string(lt.Last), lt.Purged, lt.Auction
		t.ListedAt = ""
		if lt.ListedAt != nil {
			if _, ok := parseTime(*lt.ListedAt); ok {
				t.ListedAt = *lt.ListedAt
			}
		}
		if lt.Rank != nil && *lt.Rank >= 0 && *lt.Rank <= m.opts.Atlas.Meta.Minted {
			t.Rank = *lt.Rank
		}
		if lt.Img != "" && lt.Img != m.opts.Atlas.Tokens[i].Img {
			m.artChanged[lt.ID] = true
		}
	}
	m.marketAt = time.Now()
	if timed {
		m.snapAt, m.lastEvent = at, at
	}
	m.applyEvents(m.recent)
	m.marketChanged()
	return true
}

// applyEvents applies listings, delistings, sales and bids newer than the last applied
// data. A new bid only ever raises the best one; the next snapshot catches withdrawn bids.
func (m *Model) applyEvents(evs []market.Event) bool {
	type timed struct {
		e  market.Event
		at time.Time
	}
	var todo []timed
	for i := len(evs) - 1; i >= 0; i-- {
		if at, ok := parseTime(evs[i].Time); ok && at.After(m.lastEvent) {
			todo = append(todo, timed{evs[i], at})
		}
	}
	sort.SliceStable(todo, func(a, b int) bool { return todo[a].at.Before(todo[b].at) })
	changed := false
	for _, x := range todo {
		e := x.e
		m.lastEvent = x.at
		i, ok := m.byID[e.ID]
		if !ok {
			continue
		}
		t := &m.tokens[i]
		switch e.Type {
		case "listed":
			t.Listed, t.Ask, t.ListedAt = true, string(e.Price), e.Time
		case "delisted":
			t.Listed, t.Ask, t.ListedAt = false, "", ""
		case "sold":
			t.Listed, t.Ask, t.ListedAt, t.Last = false, "", "", string(e.Price)
		case "offer-made":
			p, ok := search.Price(string(e.Price))
			if best, has := search.Price(t.Bid); !ok || has && best >= p {
				continue
			}
			t.Bid = string(e.Price)
		default:
			continue
		}
		changed = true
	}
	return changed
}

func (m *Model) marketChanged() {
	m.computeFloors()
	m.refresh(true)
}

// computeFloors finds the cheapest listing overall and per trait value.
func (m *Model) computeFloors() {
	for k := range m.floors {
		m.floors[k] = map[string]float64{}
	}
	m.floor = 0
	for i := range m.tokens {
		t := &m.tokens[i]
		if !t.Listed || t.Purged {
			continue
		}
		p, ok := search.Price(t.Ask)
		if !ok {
			continue
		}
		if m.floor == 0 || p < m.floor {
			m.floor = p
		}
		for k, v := range t.Traits {
			if f, ok := m.floors[k][v]; !ok || p < f {
				m.floors[k][v] = p
			}
		}
	}
}

func (m *Model) marketPart(words ...string) line {
	var r row
	for i, s := range words {
		if i%2 == 0 {
			r.put(fg(m.theme.MarketLabel), s)
		} else {
			r.put(fg(m.theme.Market), s)
		}
	}
	return r.line()
}

// marketParts is the market strip's summary, muted words around slightly brighter values.
func (m *Model) marketParts() []line {
	if !m.live() {
		return nil
	}
	part := m.marketPart
	var parts []line
	if m.floor > 0 {
		parts = append(parts, part("floor ", zecFloat(m.floor)+" ZEC"))
	}
	if s := m.stats; s != nil {
		if v := statZec(string(s.Volume24h)); v != "" {
			parts = append(parts, part("24h: ", itoa(s.Sales24h), " sales, ", v))
		} else {
			parts = append(parts, part("24h: ", itoa(s.Sales24h), " sales"))
		}
	}
	if m.rate != nil && m.rate.ZecUsd > 0 {
		if v := usd(m.rate.ZecUsd); v != "" {
			parts = append(parts, part("1 ZEC = ", v))
		}
	}
	return parts
}

func (m *Model) statusLine() line { return m.statusLines()[0] }

func (m *Model) statusLines() []line {
	parts := func(style string, ss ...string) []line {
		var l []line
		for _, s := range ss {
			l = append(l, text(style, s))
		}
		return l
	}
	if !m.online() {
		return []line{m.tildes(parts(fg(m.theme.Status), "offline", "prices as of "+snapshotDate(m.opts.Atlas)), m.w)}
	}
	if m.refreshing() {
		return []line{text(fg(m.theme.Busy), "refreshing...")}
	}
	if !m.live() || m.failing() {
		since := snapshotDate(m.opts.Atlas)
		if m.feeds[feedSnapshot].applied {
			since = m.marketAt.Format("15:04")
		}
		what := parts(fg(m.theme.Warn), "can't reach Zilkroad", "prices as of "+since)
		full := append(append([]line{}, what...), m.hints("r", "retry"))
		return []line{m.tildes(full, m.w), m.tildes(what, m.w), what[0]}
	}
	l := []line{m.marketPart("works ", m.worksText())}
	if v := m.volumeText(); v != "" {
		l = append(l, m.marketPart("volume ", v))
	}
	return []line{m.tildes(l, m.w)}
}

func (m *Model) worksText() string {
	if s := m.stats; s != nil && s.Supply > 0 {
		return comma(s.Supply)
	}
	return comma(m.opts.Atlas.Meta.Supply)
}

func (m *Model) volumeText() string {
	if s := m.stats; s != nil {
		if v, ok := search.Price(string(s.VolumeTotal)); ok {
			if n, ok := roundInt(v); ok {
				return comma(n) + " ZEC"
			}
		}
	}
	return ""
}

func snapshotDate(a *atlas.Atlas) string {
	if t, ok := parseTime(a.Meta.SnapshotAt); ok {
		return t.Local().Format("Jan 2")
	}
	return "build"
}
