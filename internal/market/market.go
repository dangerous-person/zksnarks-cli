// Package market fetches live market data from zilkroad.com. Every request is about the
// whole collection (snapshot, stats, events, rate); the only per-token request is History,
// and the app makes it only when the user asks for it.
package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Origin    = "https://zilkroad.com"
	userAgent = "zksnarks-cli/0.1 (unofficial; +https://github.com/dangerous-person/zksnarks-cli)"
)

// Decimal is a ZEC amount; the API writes amounts as strings, sometimes as numbers or null.
type Decimal string

func (d *Decimal) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*d = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*d = Decimal(s)
		return nil
	}
	*d = Decimal(b)
	return nil
}

// Token is the market state of one token in a snapshot.
type Token struct {
	ID       int     `json:"id"`
	Img      string  `json:"img"`
	Rank     *int    `json:"rank"`
	Listed   bool    `json:"listed"`
	Ask      Decimal `json:"ask"`
	Bid      Decimal `json:"bid"` // best open bid
	Last     Decimal `json:"last"`
	ListedAt *string `json:"listedAt"`
	Purged   bool    `json:"purged"`
	Auction  bool    `json:"auction"`
}

// Snapshot is /api/market/tokens: every token in one response.
type Snapshot struct {
	At      string  `json:"at"`
	Version string  `json:"version"`
	Tokens  []Token `json:"tokens"`
}

// Stats is /api/market/stats.
type Stats struct {
	Floor       Decimal `json:"floorZec"`
	Sales24h    int     `json:"sales24h"`
	Volume24h   Decimal `json:"volume24hZec"`
	VolumeTotal Decimal `json:"volumeTotalZec"`
	Supply      int     `json:"supply"`
	Purged      int     `json:"purged"`
	UpdatedAt   string  `json:"updatedAt"`
}

// Rate is /api/market/rate.
type Rate struct {
	ZecUsd float64 `json:"zecUsd"`
	At     string  `json:"at"`
	Stale  bool    `json:"stale"`
}

// Event is one entry of /api/market/events.
type Event struct {
	ID    int     `json:"id"`
	Type  string  `json:"type"` // listed, delisted, sold, offer-made
	Price Decimal `json:"price"`
	Time  string  `json:"time"`
}

// HistoryEntry is one line of a token's provenance from /api/snarks/{id}.
type HistoryEntry struct {
	Type      string  `json:"type"` // snark_minted, listing_created, listing_cancelled, ...
	Amount    Decimal `json:"amountZec"`
	CreatedAt string  `json:"createdAt"`
}

// HistoryCap is how many entries /api/snarks/{id} returns at most: the oldest ones, with no
// way to page further, so a busy token's newest activity is missing.
const HistoryCap = 200

// historyNames are the labels the site gives the entry types, a few shortened to fit a
// terminal line.
var historyNames = map[string]string{
	"apparition_unlocked": "Unveiled",
	"snark_purged":        "Purged",
	"snark_minted":        "Minted",
	"snark_transferred":   "Transferred",
	"snark_traded":        "Traded",
	"snark_auctioned":     "Auctioned",
	"listing_created":     "Listed",
	"listing_cancelled":   "Delisted",
	"order_paid":          "Sold",
	"offer_accepted":      "Sold",
	"delivery_queued":     "Delivered",
	"offer_created":       "Offer made",
	"offer_withdrawn":     "Offer removed",
	"payout":              "Payout sent",
	"list":                "Listed",
	"delist":              "Delisted",
	"transfer":            "Transferred",
	"sell":                "Sold",
}

// Name is a short, human name for the entry type.
func (h HistoryEntry) Name() string {
	if n, ok := historyNames[h.Type]; ok {
		return n
	}
	return strings.ReplaceAll(h.Type, "_", " ")
}

// Counter tracks network use for the status line.
type Counter struct {
	requests atomic.Int64
	perToken atomic.Int64
	mu       sync.Mutex
	last     time.Time
}

func (c *Counter) add(perToken bool) {
	c.requests.Add(1)
	if perToken {
		c.perToken.Add(1)
	}
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
}

// Requests returns the number of requests, per-token requests and the last request time.
func (c *Counter) Requests() (total, perToken int, last time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int(c.requests.Load()), int(c.perToken.Load()), c.last
}

// Client makes the requests. A nil Client, or one with Offline set, makes none.
type Client struct {
	HTTP    *http.Client
	Offline bool
	Net     Counter
}

// New returns a client with sensible timeouts.
func New(offline bool) *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, Offline: offline}
}

// ErrOffline is returned by every call of an offline client.
var ErrOffline = fmt.Errorf("offline")

func (c *Client) get(ctx context.Context, path string, perToken bool, v any) error {
	if c == nil || c.Offline {
		return ErrOffline
	}
	req, err := http.NewRequestWithContext(ctx, "GET", Origin+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	c.Net.add(perToken)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Snapshot fetches all tokens (about 300 KB compressed).
func (c *Client) Snapshot(ctx context.Context) (*Snapshot, error) {
	var s Snapshot
	if err := c.get(ctx, "/api/market/tokens", false, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Stats fetches the collection stats.
func (c *Client) Stats(ctx context.Context) (*Stats, error) {
	var s Stats
	if err := c.get(ctx, "/api/market/stats", false, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Rate fetches the ZEC/USD rate.
func (c *Client) Rate(ctx context.Context) (*Rate, error) {
	var r Rate
	if err := c.get(ctx, "/api/market/rate", false, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Events fetches the latest market events, newest first.
func (c *Client) Events(ctx context.Context) ([]Event, error) {
	var e struct {
		Events []Event `json:"events"`
	}
	if err := c.get(ctx, "/api/market/events", false, &e); err != nil {
		return nil, err
	}
	return e.Events, nil
}

// History fetches one token's provenance, oldest first. This request tells the server which
// token the user is interested in, so the app only makes it on an explicit key press.
func (c *Client) History(ctx context.Context, id int) ([]HistoryEntry, error) {
	var r struct {
		History []HistoryEntry `json:"history"`
	}
	if err := c.get(ctx, "/api/snarks/"+strconv.Itoa(id), true, &r); err != nil {
		return nil, err
	}
	return r.History, nil
}
