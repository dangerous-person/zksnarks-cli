//go:build dev

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m|\x1b\]8;[^\x1b]*\x1b\\`)

// go test -tags dev -run TestShots ./internal/ui   writes frames to $SHOTS for review
func TestShots(t *testing.T) {
	dir := os.Getenv("SHOTS")
	a, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	content, err := embedded.LoadSite()
	if err != nil {
		t.Fatal(err)
	}
	type step struct {
		keys string // space separated key names; "type:..." types text
		shot string
	}
	scenarios := []struct {
		name  string
		w, h  int
		query string
		steps []step
	}{
		{"main", 120, 40, "", []step{
			{"", "gallery"},
			{"j j l", "gallery-moved"},
			{"/ type:skele type:space type:rim", "search-typing"},
			{"tab", "search-completed"},
			{"enter", "search-done"},
			{"f", "filters"},
			{"type:c type:h type:a type:r", "filters-find"},
			{"esc space", "filters-toggled"},
			{"esc", "filters-closed"},
			{"enter", "card"},
			{"H", "card-history-offline"},
			{"esc esc enter", "card-plain"},
			{"right right", "card-next"},
			{"esc ?", "help"},
		}},
		{"small", 80, 24, "", []step{{"", "gallery-80x24"}, {"enter", "card-80x24"}}},
		{"min", 60, 20, "lineage:skele sort:price", []step{{"", "gallery-60x20"}, {"enter", "card-60x20"}}},
		{"tiny", 59, 20, "", []step{{"", "too-small"}}},
		{"modal-160", 160, 48, "", []step{{"sel:63 enter", "modal63"}, {"esc sel:7724 enter", "modal7724"}, {"esc sel:9701 enter", "modal-apparition"}}},
		{"green-160", 160, 48, "", []step{{"t", "green-gallery"}, {"sel:63 enter", "green-modal63"}}},
		{"sections", 120, 40, "", []step{
			{"2", "luminaries"},
			{"pgdown", "luminaries-scrolled"},
			{"3", "network"},
			{"down", "network-2"},
			{"o", "network-open"},
		}},
		{"sections-small", 80, 24, "", []step{{"2", "luminaries"}, {"3", "network"}}},
		{"sections-min", 60, 20, "", []step{{"2", "luminaries"}, {"3", "network"}}},
		{"sections-wide", 180, 50, "", []step{{"2", "luminaries"}, {"3", "network"}}},
		{"luminaries-wide", 250, 64, "", []step{{"2", "luminaries"}, {"G", "luminaries-end"}}},
		{"luminaries-edge", 220, 60, "", []step{{"2", "luminaries"}}},
		{"purged", 120, 40, "purged sort:rank", []step{{"", "gallery-purged"}, {"enter", "card-purged"}, {"a", "card-apparition"}}},
		{"wide", 180, 50, "listed sort:price", []step{{"", "gallery-180x50"}, {"enter", "card-180x50"}}},
		{"provenance", 160, 48, "", []step{
			{"sel:161 enter", "prov-none"},
			{"fakehist", "prov-latest"},
			{"H", "prov-full"},
			{"down down down", "prov-scrolled"},
			{"H", "prov-closed"},
			{"right H", "prov-offline"},
		}},
		{"bid", 160, 48, "", []step{{"sel:69 enter fakebid", "bid69"}, {"esc sel:9701 enter fakebid", "bid-apparition"}}},
		{"provenance-small", 80, 24, "", []step{{"sel:161 enter fakehist", "prov-80x24"}, {"H", "prov-full-80x24"}}},
		{"network-wide", 250, 64, "", []step{{"3", "network"}, {"down", "network-josh"}}},
	}
	for _, sc := range scenarios {
		m := New(Options{Atlas: a, Site: content, Glyph: art.UpperHalf, Client: market.New(true), State: State{Query: sc.query, Section: "zilkroad"}})
		m.Update(tea.WindowSizeMsg{Width: sc.w, Height: sc.h})
		for _, st := range sc.steps {
			for _, k := range strings.Fields(st.keys) {
				if k == "fakebid" { // a listing and a bid on the selected token
					if tk, ok := m.selected(); ok {
						tk.Listed, tk.Ask, tk.ListedAt, tk.Bid = true, "8", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), "5"
					}
					continue
				}
				if k == "fakehist" { // a loaded history with every entry type
					if tk, ok := m.selected(); ok {
						m.histories[tk.ID] = &history{entries: fakeHistory(time.Now())}
					}
					continue
				}
				if id, ok := strings.CutPrefix(k, "sel:"); ok { // select a token by number
					for i, r := range m.results {
						if itoa(m.tokens[r].ID) == id {
							m.sel = i
						}
					}
					m.ensureVisible()
					continue
				}
				m.Update(press(k))
			}
			before := truncations
			content := m.View().Content
			if truncations != before {
				t.Errorf("%s: %d lines cut to fit", st.shot, truncations-before)
			}
			lines := strings.Split(content, "\n")
			if sc.w >= minW && len(lines) != sc.h {
				t.Errorf("%s: %d lines, want %d", st.shot, len(lines), sc.h)
			}
			for i, l := range lines {
				if n := utf8.RuneCountInString(ansiRe.ReplaceAllString(l, "")); sc.w >= minW && n != sc.w {
					t.Errorf("%s line %d: width %d, want %d", st.shot, i, n, sc.w)
				}
			}
			name := fmt.Sprintf("%s_%dx%d", st.shot, sc.w, sc.h)
			os.WriteFile(filepath.Join(dir, name+".ans"), []byte(content), 0o644)
			os.WriteFile(filepath.Join(dir, name+".txt"), []byte(ansiRe.ReplaceAllString(content, "")), 0o644)
		}
	}
}

// fakeHistory has every entry type the site knows, oldest first as the API sends them.
func fakeHistory(now time.Time) []market.HistoryEntry {
	types := []struct {
		typ, amount string
		ago         time.Duration
	}{
		{"snark_minted", "", 720 * time.Hour},
		{"listing_created", "19", 700 * time.Hour},
		{"offer_created", "3.5", 500 * time.Hour},
		{"offer_withdrawn", "", 480 * time.Hour},
		{"order_paid", "4", 360 * time.Hour},
		{"snark_transferred", "", 359 * time.Hour},
		{"delivery_queued", "", 358 * time.Hour},
		{"payout", "3.9", 357 * time.Hour},
		{"listing_created", "30", 216 * time.Hour},
		{"listing_cancelled", "", 215 * time.Hour},
		{"listing_created", "14.5", 192 * time.Hour},
		{"listing_cancelled", "", 145 * time.Hour},
		{"snark_traded", "", 144 * time.Hour},
		{"offer_accepted", "9", 143 * time.Hour},
		{"snark_auctioned", "12", 140 * time.Hour},
		{"apparition_unlocked", "", 100 * time.Hour},
		{"snark_purged", "", 99 * time.Hour},
		{"listing_created", "9.1", 144 * time.Hour},
		{"listing_cancelled", "", 26 * time.Hour},
		{"listing_created", "8", 25 * time.Hour},
	}
	var out []market.HistoryEntry
	for _, t := range types {
		out = append(out, market.HistoryEntry{Type: t.typ, Amount: market.Decimal(t.amount), CreatedAt: now.Add(-t.ago).UTC().Format(time.RFC3339Nano)})
	}
	return out
}

func press(k string) tea.KeyPressMsg {
	if strings.HasPrefix(k, "type:") {
		s := strings.TrimPrefix(k, "type:")
		if s == "space" {
			return tea.KeyPressMsg{Code: ' ', Text: " "}
		}
		r, _ := utf8.DecodeRuneInString(s)
		_ = r
		return tea.KeyPressMsg{Code: r, Text: s}
	}
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}
