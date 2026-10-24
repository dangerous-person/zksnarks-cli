package ui

import (
	"context"
	"math"
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
)

type (
	snapshotMsg struct {
		s   *market.Snapshot
		err error
	}
	statsMsg struct {
		s   *market.Stats
		err error
	}
	rateMsg struct {
		r   *market.Rate
		err error
	}
	eventsMsg struct {
		e   []market.Event
		err error
	}
	historyMsg struct {
		id  int
		e   []market.HistoryEntry
		err error
	}
	tickEventsMsg   struct{}
	tickSnapshotMsg struct{}
	tickRateMsg     struct{}
)

func tickEvents() tea.Cmd {
	return tea.Tick(eventsEvery, func(time.Time) tea.Msg { return tickEventsMsg{} })
}

func tickSnapshot() tea.Cmd {
	return tea.Tick(snapshotEvery, func(time.Time) tea.Msg { return tickSnapshotMsg{} })
}

func tickRate() tea.Cmd {
	return tea.Tick(rateEvery, func(time.Time) tea.Msg { return tickRateMsg{} })
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func (m *Model) fetchSnapshot() tea.Cmd {
	c := m.opts.Client
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		s, err := c.Snapshot(cx)
		return snapshotMsg{s, err}
	}
}

func (m *Model) fetchStats() tea.Cmd {
	c := m.opts.Client
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		s, err := c.Stats(cx)
		return statsMsg{s, err}
	}
}

func (m *Model) fetchRate() tea.Cmd {
	c := m.opts.Client
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		r, err := c.Rate(cx)
		return rateMsg{r, err}
	}
}

func (m *Model) fetchEvents() tea.Cmd {
	c := m.opts.Client
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		e, err := c.Events(cx)
		return eventsMsg{e, err}
	}
}

func (m *Model) online() bool { return m.opts.Client != nil && !m.opts.Client.Offline }

func (m *Model) refreshMarket() tea.Cmd {
	if !m.online() {
		return nil
	}
	return tea.Batch(m.fetchSnapshot(), m.fetchStats(), m.fetchRate())
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
	case snapshotMsg:
		if msg.err != nil {
			m.marketErr = msg.err
			return nil
		}
		m.marketErr = nil
		m.applySnapshot(msg.s)
	case statsMsg:
		if msg.err == nil {
			m.stats = msg.s
		}
	case rateMsg:
		if msg.err == nil {
			m.rate = msg.r
		}
	case eventsMsg:
		if msg.err != nil {
			m.marketErr = msg.err
			return nil
		}
		m.marketErr = nil // the market answers again, so the next snapshot need not be waited for
		m.applyEvents(msg.e)
	case historyMsg:
		m.histories[msg.id] = &history{entries: msg.e, err: msg.err}
	case tickEventsMsg:
		return tea.Batch(m.fetchEvents(), tickEvents())
	case tickSnapshotMsg:
		return tea.Batch(m.fetchSnapshot(), m.fetchStats(), tickSnapshot())
	case tickRateMsg:
		return tea.Batch(m.fetchRate(), tickRate())
	}
	return nil
}

func (m *Model) applySnapshot(s *market.Snapshot) {
	for _, lt := range s.Tokens {
		i, ok := m.byID[lt.ID]
		if !ok {
			continue
		}
		t := &m.tokens[i]
		if t.Purged != lt.Purged {
			delete(m.thumbs, i) // purged art is drawn in grey
			delete(m.cards, i)
		}
		t.Listed, t.Ask, t.Bid, t.Last, t.Purged, t.Auction = lt.Listed, string(lt.Ask), string(lt.Bid), string(lt.Last), lt.Purged, lt.Auction
		t.ListedAt = ""
		if lt.ListedAt != nil {
			t.ListedAt = *lt.ListedAt
		}
		if lt.Rank != nil {
			t.Rank = *lt.Rank
		}
		if lt.Img != "" && lt.Img != m.opts.Atlas.Tokens[i].Img {
			m.artChanged[lt.ID] = true
		}
	}
	m.marketAt = time.Now()
	if m.lastEvent == "" || s.At > m.lastEvent {
		m.lastEvent = s.At
	}
	m.marketChanged()
}

// applyEvents applies listings, delistings, sales and bids newer than the last applied
// data. A new bid only ever raises the best one; the next snapshot catches withdrawn bids.
func (m *Model) applyEvents(evs []market.Event) {
	sort.SliceStable(evs, func(a, b int) bool { return evs[a].Time < evs[b].Time })
	changed := false
	for _, e := range evs {
		if e.Time <= m.lastEvent {
			continue
		}
		m.lastEvent = e.Time
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
	if changed {
		m.marketChanged()
	}
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
	part := m.marketPart
	var parts []line
	if m.floor > 0 {
		parts = append(parts, part("floor ", zecFloat(m.floor)+" ZEC"))
	}
	if m.stats != nil {
		parts = append(parts, part("24h: ", itoa(m.stats.Sales24h), " sales, ", zecShort(string(m.stats.Volume24h))+" ZEC"))
	}
	if m.rate != nil && m.rate.ZecUsd > 0 {
		parts = append(parts, part("1 ZEC = ", usd(m.rate.ZecUsd)))
	}
	return parts
}

func zecShort(s string) string {
	p, ok := search.Price(s)
	if !ok {
		return s
	}
	return trimFloat(p)
}

func (m *Model) statusLine() line {
	parts := func(style string, ss ...string) line {
		var l []line
		for _, s := range ss {
			l = append(l, text(style, s))
		}
		return m.tildes(l, m.w)
	}
	if !m.online() {
		return parts(fg(m.theme.Status), "offline", "prices as of "+snapshotDate(m.opts.Atlas))
	}
	if m.marketErr != nil {
		since := "build"
		if !m.marketAt.IsZero() {
			since = m.marketAt.Format("15:04")
		}
		return parts(fg(m.theme.Warn), "market unreachable", "prices as of "+since)
	}
	l := []line{m.marketPart("works ", m.worksText())}
	if v := m.volumeText(); v != "" {
		l = append(l, m.marketPart("volume ", v))
	}
	return m.tildes(l, m.w)
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
			return comma(int(math.Round(v))) + " ZEC"
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
