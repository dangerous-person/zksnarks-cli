package ui

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/media"
)

func TestSnapshotRank(t *testing.T) {
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{Atlas: a, Client: market.New(true)})
	i := m.byID[907]
	baked := m.tokens[i].Rank
	cases := []struct {
		rank, want int
	}{
		{-1, baked},
		{math.MinInt64, baked},
		{a.Meta.Minted + 1, baked},
		{math.MaxInt64, baked},
		{42, 42},
		{a.Meta.Minted, a.Meta.Minted},
		{0, 0},
	}
	for _, c := range cases {
		m.tokens[i].Rank = baked
		r := c.rank
		m.applySnapshot(&market.Snapshot{Tokens: []market.Token{{ID: 907, Rank: &r}}})
		if got := m.tokens[i].Rank; got != c.want {
			t.Errorf("rank %d: got %d, want %d", c.rank, got, c.want)
		}
	}
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func bare(l line) string { return sgr.ReplaceAllString(l.s, "") }

func onlineModel(t *testing.T) *Model {
	t.Helper()
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{Atlas: a, Client: market.New(false), Media: media.FS, State: State{Section: "zilkroad"}})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	return m
}

func answer(m *Model, k feedKind, val any, err error) {
	m.Update(feedMsg{kind: k, seq: m.feeds[k].seq, val: val, err: err})
}

func strip(m *Model) (string, string) {
	var parts []string
	for _, p := range m.marketParts() {
		parts = append(parts, bare(p))
	}
	return strings.Join(parts, " ~ "), bare(m.statusLine())
}

func TestStatusStrip(t *testing.T) {
	m := onlineModel(t)
	built := snapshotDate(m.opts.Atlas)
	down := errors.New("connection refused")
	errText := "can't reach zilkroad ~ prices as of " + built + " ~ [R] RETRY"
	check := func(step, left, right string) {
		t.Helper()
		l, r := strip(m)
		if l != left || r != right {
			t.Errorf("%s: strip %q | %q, want %q | %q", step, l, r, left, right)
		}
	}
	check("before any fetch", "", errText)

	m.startFeeds()
	check("first load", "", "refreshing...")
	answer(m, feedRate, &market.Rate{ZecUsd: 1190.85}, nil)
	answer(m, feedSnapshot, nil, down)
	answer(m, feedStats, nil, down)
	check("start failed", "", errText)

	m.fetch(feedEvents)
	check("events in flight", "", errText)
	answer(m, feedEvents, []market.Event{}, nil)
	check("events answer", "", errText)

	if m.retryMarket() == nil {
		t.Fatal("r made no requests")
	}
	seqs := m.feeds
	check("retry", "", "refreshing...")
	if m.retryMarket() != nil || m.feeds != seqs {
		t.Fatal("a second r while the first is in flight made requests")
	}
	answer(m, feedSnapshot, &market.Snapshot{At: "2026-10-24T21:21:06Z", Tokens: []market.Token{{ID: 907, Listed: true, Ask: "0.25"}}}, nil)
	check("snapshot back, stats pending", "", "refreshing...")
	answer(m, feedStats, &market.Stats{Sales24h: 17, Volume24h: "10.88", VolumeTotal: "6627.73", Supply: 9324}, nil)
	answer(m, feedRate, &market.Rate{ZecUsd: 1190.85}, nil)
	check("rate pending", "floor 0.25 ZEC ~ 24h: 17 sales, 10.88 ZEC ~ 1 ZEC = $1,191", "refreshing...")
	answer(m, feedEvents, []market.Event{}, nil)
	live := "floor 0.25 ZEC ~ 24h: 17 sales, 10.88 ZEC ~ 1 ZEC = $1,191"
	check("live", live, "works 9,324 ~ volume 6,628 ZEC")

	m.fetch(feedEvents)
	check("background poll", live, "works 9,324 ~ volume 6,628 ZEC")
	answer(m, feedEvents, nil, down)
	check("poll failed", live, "can't reach zilkroad ~ prices as of "+m.marketAt.Format("15:04")+" ~ [R] RETRY")
	m.fetch(feedEvents)
	check("retry in background", live, "can't reach zilkroad ~ prices as of "+m.marketAt.Format("15:04")+" ~ [R] RETRY")
	answer(m, feedEvents, nil, market.ErrNotModified)
	check("not modified", live, "works 9,324 ~ volume 6,628 ZEC")

	m.fetch(feedSnapshot)
	answer(m, feedSnapshot, nil, down)
	m.fetch(feedStats)
	answer(m, feedStats, nil, down)
	check("refresh failed", live, "can't reach zilkroad ~ prices as of "+m.marketAt.Format("15:04")+" ~ [R] RETRY")
}

func TestStatusStripOffline(t *testing.T) {
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{Atlas: a, Client: market.New(true), Media: media.FS})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if m.Init(); m.fetching() {
		t.Fatal("offline model fetches")
	}
	if m.retryMarket() != nil {
		t.Fatal("r makes requests offline")
	}
	l, r := strip(m)
	if want := "offline ~ prices as of " + snapshotDate(a); l != "" || r != want {
		t.Fatalf("strip %q | %q, want empty | %q", l, r, want)
	}
}

func TestFeedOrder(t *testing.T) {
	m := onlineModel(t)
	i := m.byID[907]
	snap := func(at, ask string) *market.Snapshot {
		return &market.Snapshot{At: at, Tokens: []market.Token{{ID: 907, Listed: true, Ask: market.Decimal(ask)}}}
	}
	m.fetch(feedSnapshot)
	m.fetch(feedSnapshot)
	m.Update(feedMsg{kind: feedSnapshot, seq: 2, val: snap("2026-10-24T12:00:00Z", "2"), etag: `"b"`})
	m.Update(feedMsg{kind: feedSnapshot, seq: 1, val: snap("2026-10-24T13:00:00Z", "1"), etag: `"a"`})
	if got := m.tokens[i].Ask; got != "2" {
		t.Fatalf("an older request's answer won: ask %q", got)
	}
	if f := m.feeds[feedSnapshot]; f.inflight || f.etag != `"b"` {
		t.Fatalf("feed after answers: %+v", f)
	}
	m.fetch(feedSnapshot)
	answer(m, feedSnapshot, snap("2026-10-24T11:00:00Z", "3"), nil)
	if got := m.tokens[i].Ask; got != "2" {
		t.Fatalf("an older snapshot won: ask %q", got)
	}
	if f := m.feeds[feedSnapshot]; f.inflight || f.failed || f.etag != `"b"` {
		t.Fatalf("feed after an old snapshot: %+v", f)
	}
	before := m.marketAt
	m.fetch(feedSnapshot)
	answer(m, feedSnapshot, nil, market.ErrNotModified)
	if got := m.tokens[i].Ask; got != "2" || !m.marketAt.After(before) {
		t.Fatalf("not modified: ask %q, snapshot time %v after %v", got, m.marketAt, before)
	}

	sale := market.Event{ID: 907, Type: "sold", Price: "1.5", Time: "2026-10-24T12:02:00Z"}
	m.fetch(feedEvents)
	answer(m, feedEvents, []market.Event{sale}, nil)
	if tk := m.tokens[i]; tk.Listed || tk.Last != "1.5" {
		t.Fatalf("sale not applied: %+v", tk)
	}
	m.fetch(feedSnapshot)
	answer(m, feedSnapshot, snap("2026-10-24T12:01:00Z", "2"), nil)
	if tk := m.tokens[i]; tk.Listed || tk.Last != "1.5" {
		t.Fatalf("a snapshot older than the sale reverted it: listed %v, last %q", tk.Listed, tk.Last)
	}
	if want, _ := parseTime(sale.Time); !m.lastEvent.Equal(want) {
		t.Fatalf("watermark %v, want %v", m.lastEvent, want)
	}
}

func TestEventOrder(t *testing.T) {
	m := onlineModel(t)
	i := m.byID[907]
	m.fetch(feedEvents)
	answer(m, feedEvents, []market.Event{
		{ID: 907, Type: "listed", Price: "4", Time: "2099-01-01T00:00:01Z"},
		{ID: 907, Type: "delisted", Price: "3", Time: "2099-01-01T00:00:01Z"},
		{ID: 907, Type: "listed", Price: "9", Time: "garbage"},
	}, nil)
	if tk := m.tokens[i]; !tk.Listed || tk.Ask != "4" {
		t.Fatalf("listed %v, ask %q; want the newest event of a tie, listed at 4", tk.Listed, tk.Ask)
	}
	if want, _ := parseTime("2099-01-01T00:00:01Z"); !m.lastEvent.Equal(want) {
		t.Fatalf("watermark %v, want %v", m.lastEvent, want)
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, w := range want {
		if got := backoff(i + 1); got != w {
			t.Errorf("backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	if got := backoff(1000); got != retryMax {
		t.Errorf("backoff(1000) = %v", got)
	}
	for range 1000 {
		if d := jitter(time.Minute); d < 54*time.Second || d > 66*time.Second {
			t.Fatalf("jitter(1m) = %v", d)
		}
	}
	m := onlineModel(t)
	for n := 1; n <= 3; n++ {
		m.fetch(feedEvents)
		answer(m, feedEvents, nil, errors.New("down"))
		if f := m.feeds[feedEvents]; f.fails != n || time.Until(f.due) < 9*backoff(n)/10-time.Second || time.Until(f.due) > 11*backoff(n)/10 {
			t.Fatalf("after %d failures: fails %d, next in %v", n, f.fails, time.Until(f.due))
		}
	}
	m.retryMarket()
	if f := m.feeds[feedEvents]; f.fails != 0 {
		t.Fatalf("r kept the backoff: %d failures", f.fails)
	}
	answer(m, feedEvents, []market.Event{}, nil)
	if f := m.feeds[feedEvents]; f.fails != 0 || time.Until(f.due) > 33*time.Second {
		t.Fatalf("after a success: fails %d, next in %v", f.fails, time.Until(f.due))
	}
}

func TestFocusPausesPolling(t *testing.T) {
	m := onlineModel(t)
	m.schedule(feedEvents)
	m.schedule(feedRate)
	m.Update(tea.BlurMsg{})
	if _, cmd := m.Update(feedTickMsg{feedEvents, m.feeds[feedEvents].gen}); cmd != nil || m.fetching() {
		t.Fatal("a tick while blurred fetched")
	}
	m.feeds[feedEvents].due = time.Now().Add(-time.Second)
	m.Update(tea.FocusMsg{})
	if !m.feeds[feedEvents].inflight {
		t.Fatal("focus did not fetch the overdue events")
	}
	if m.feeds[feedRate].inflight {
		t.Fatal("focus fetched the rate before it was due")
	}
	if _, cmd := m.Update(tea.FocusMsg{}); cmd != nil {
		t.Fatal("a second focus fetched again")
	}

	m = onlineModel(t)
	m.schedule(feedEvents)
	if _, cmd := m.Update(feedTickMsg{feedEvents, m.feeds[feedEvents].gen}); cmd == nil || !m.feeds[feedEvents].inflight {
		t.Fatal("without focus reports polling stopped")
	}
}

func TestBadAmountsHidden(t *testing.T) {
	m := onlineModel(t)
	i := m.byID[907]
	tk := &m.tokens[i]
	tk.Listed, tk.Ask, tk.Bid, tk.Last, tk.ListedAt, tk.Purged = true, "NaN", "Infinity", "NaN", "garbage", false
	m.histories[907] = &history{entries: []market.HistoryEntry{{Type: "order_paid", Amount: "NaN", CreatedAt: "garbage"}, {Type: "listing_created", Amount: "-Inf", CreatedAt: "2026-10-01T00:00:00Z"}}}
	m.input.set("#907")
	m.refresh(false)
	bad := []string{"NaN", "Inf", "garbage"}
	look := func(step string) {
		t.Helper()
		screen := sgr.ReplaceAllString(m.View().Content, "")
		for _, b := range bad {
			if strings.Contains(screen, b) {
				t.Errorf("%s: %q on screen", step, b)
			}
		}
	}
	m.mode = modeGallery
	look("gallery")
	m.mode = modeList
	look("list")
	m.card = true
	look("card")
	m.histFor = 907
	look("history")
	for _, s := range []string{"", "NaN", "Infinity", "abc"} {
		if got := zec(s); got != "-" {
			t.Errorf("zec(%q) = %q", s, got)
		}
		if got := lastText(s); got != "-" {
			t.Errorf("lastText(%q) = %q", s, got)
		}
	}
	if got := zec("0.5"); got != "0.50" {
		t.Errorf("zec(0.5) = %q", got)
	}
}

func TestHintsRowStatus(t *testing.T) {
	m := onlineModel(t)
	built := snapshotDate(m.opts.Atlas)
	long, short := "[ENTER] OPEN  [F] FILTERS  [V] GRID  [?] HELP", "[F] FILTERS  [?] HELP"
	full := "can't reach zilkroad ~ prices as of " + built + " ~ [R] RETRY"
	row := func(w int, hint, right string) string {
		gap := w - 2 - len(hint) - len(right)
		if right == "" {
			gap++
		}
		return " " + hint + strings.Repeat(" ", gap) + right + strings.Repeat(" ", min(1, len(right)))
	}
	cases := []struct {
		w, h        int
		hint, right string
	}{
		{140, 24, long, full},
		{80, 24, short, full},
		{70, 24, short, "can't reach zilkroad ~ prices as of " + built},
		{66, 24, short, "can't reach zilkroad"},
		{60, 20, short, "can't reach zilkroad"},
		{80, 30, long, "[T] THEME"},
	}
	for _, c := range cases {
		m.Update(tea.WindowSizeMsg{Width: c.w, Height: c.h})
		got := sgr.ReplaceAllString(m.hintsRow(), "")
		if want := row(c.w, c.hint, c.right); got != want || width(got) != c.w {
			t.Errorf("%dx%d: hints row\n%q, want\n%q", c.w, c.h, got, want)
		}
	}
}
