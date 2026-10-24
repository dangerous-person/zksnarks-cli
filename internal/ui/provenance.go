package ui

import (
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
)

const histPage = 10 // lines a page key scrolls the full history by

// toggleHistory opens the selected token's full history in the card, loading it first if
// needed, or closes it again; while the history failed to load it tries again instead.
func (m *Model) toggleHistory() tea.Cmd {
	t, ok := m.selected()
	if !ok {
		return nil
	}
	if h := m.histories[t.ID]; m.histFor == t.ID && (h == nil || h.err == nil) {
		m.histFor = 0
		return nil
	}
	m.histFor, m.histTop = t.ID, 0
	return m.loadHistory()
}

// events is the token's loaded history, newest first like the site's. The server sends only
// the oldest entries of a busy token, so a live listing newer than all of them is put first.
func (m *Model) events(t *atlas.Token, h *history) []market.HistoryEntry {
	out := make([]market.HistoryEntry, 0, len(h.entries)+1)
	for i := len(h.entries) - 1; i >= 0; i-- {
		out = append(out, h.entries[i])
	}
	at := func(e market.HistoryEntry) time.Time {
		t, _ := parseTime(e.CreatedAt)
		return t
	}
	sort.SliceStable(out, func(a, b int) bool { return at(out[a]).After(at(out[b])) })
	if la, ok := parseTime(t.ListedAt); ok && t.Listed && t.Ask != "" && (len(out) == 0 || la.After(at(out[0]).Add(time.Minute))) {
		live := market.HistoryEntry{Type: "listing_created", Amount: market.Decimal(t.Ask), CreatedAt: t.ListedAt}
		out = append([]market.HistoryEntry{live}, out...)
	}
	return out
}

// eventRow is one history line: the label, how long ago, and the amount at the right.
func (m *Model) eventRow(e market.HistoryEntry, w int, now time.Time) line {
	var r row
	labelW := min(14, max(4, w-1-8-1-9))
	r.put(fg(m.theme.Text), clip(e.Name(), labelW))
	r.padTo(labelW + 2)
	if at, ok := parseTime(e.CreatedAt); ok {
		r.put(fg(m.theme.Dim), clip(ago(at, now), max(0, w-r.w)))
	}
	amount, style := "-", fg(m.theme.Dim)
	if e.Amount != "" {
		amount, style = zec(string(e.Amount))+" ZEC", fg(m.theme.Text)
	}
	if r.w+1+width(amount) <= w {
		r.padTo(w - width(amount))
		r.put(style, amount)
	}
	return r.line()
}

// historyView is the token's full history, in place of its details: the heading, a gap, as
// many entries as fit in h lines (only they scroll, by histTop), a gap and the key that
// closes it. (ox, oy) is where the view starts on the screen, for the click target.
func (m *Model) historyView(t *atlas.Token, w, h, ox, oy int) []line {
	dim := fg(m.theme.Dim)
	keyLabel := [3]string{"h", "hide history", "hide"}
	hist := m.histories[t.ID]
	var body []line
	right := ""
	switch {
	case hist == nil || hist.loading:
		body = append(body, text(dim, "loading history…"))
	case hist.err != nil:
		body = append(body, text(fg(m.theme.Warn), clip("history: "+hist.err.Error(), w)))
		keyLabel = [3]string{"h", "try again", "retry"}
	default:
		evs := m.events(t, hist)
		if len(evs) == 0 {
			body = append(body, text(dim, clip("No activity recorded yet.", w)))
			break
		}
		now := time.Now()
		var all []line
		for _, e := range evs {
			all = append(all, m.eventRow(e, w, now))
		}
		if len(hist.entries) >= market.HistoryCap {
			all = append(all, line{}, text(dim, clip("The server sends only the first "+itoa(market.HistoryCap)+" entries.", w)))
		}
		rows := max(1, h-4) // the heading, two gaps and the key
		m.histTop = max(0, min(m.histTop, len(all)-rows))
		body = all[m.histTop:min(len(all), m.histTop+rows)]
		right = plural(len(evs), "event")
		if len(all) > rows {
			right = itoa(m.histTop+1) + "-" + itoa(min(len(evs), m.histTop+rows)) + " of " + itoa(len(evs))
		}
	}
	out := []line{leftRight(bold(m.theme.Text), "PROVENANCE_", dim, right, w), {}}
	out = append(out, body...)
	for len(out) < h-1 {
		out = append(out, line{})
	}
	return append(out, m.cardKeys(len(out), ox, oy, w, keyLabel))
}

// plural writes a count with its noun, adding an s unless the count is one.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return itoa(n) + " " + noun + "s"
}
