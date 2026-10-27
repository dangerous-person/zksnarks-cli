// Package market fetches live market data from zilkroad.com. Every request is about the
// whole collection (snapshot, stats, events, rate); the only per-token request is History,
// and the app makes it only when the user asks for it.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const Origin = "https://zilkroad.com"

var Version = "dev"

func userAgent() string {
	return "zksnarks-cli/" + Version + " (unofficial; +https://github.com/dangerous-person/zksnarks-cli)"
}

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
		*d = decimal(s)
		return nil
	}
	*d = decimal(string(b))
	return nil
}

func decimal(s string) Decimal {
	if len(s) > 32 {
		return ""
	}
	dot, whole := false, 0
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
			if !dot {
				whole++
			}
		case c == '.' && !dot && i > 0 && i < len(s)-1:
			dot = true
		default:
			return ""
		}
	}
	if whole > 9 {
		return ""
	}
	for len(s) > 1 && s[0] == '0' && s[1] != '.' {
		s = s[1:]
	}
	return Decimal(s)
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
	MarketCap   Decimal `json:"marketCapZec"`
	SalesTotal  int     `json:"salesTotal"`
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

// Sale is one entry of /api/market/sales.
type Sale struct {
	ID    int     `json:"id"`
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
	return strings.Map(func(r rune) rune {
		switch {
		case r == '_':
			return ' '
		case r < ' ' || r > '~':
			return -1
		}
		return r
	}, h.Type)
}

// Client makes the requests. A nil Client, or one with Offline set, makes none.
type Client struct {
	HTTP    *http.Client
	Offline bool
}

const (
	snapshotCap  = 16 << 20
	bodyCap      = 1 << 20
	maxRedirects = 3
)

// New returns a client with sensible timeouts.
func New(offline bool) *Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = nil
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second, Transport: t, CheckRedirect: checkRedirect}, Offline: offline}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	o, err := url.Parse(Origin)
	if err != nil {
		return err
	}
	if req.URL.Scheme != "https" || req.URL.Host != o.Host {
		return fmt.Errorf("refused a redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// ErrOffline is returned by every call of an offline client.
var ErrOffline = fmt.Errorf("offline")

var ErrNotModified = errors.New("not modified")

func (c *Client) get(ctx context.Context, path, etag string, limit int64, v any) (string, error) {
	if c == nil || c.Offline {
		return "", ErrOffline
	}
	req, err := http.NewRequestWithContext(ctx, "GET", Origin+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent())
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && etag != "" {
		return etag, ErrNotModified
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, limit))
		return "", fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > limit {
		return "", fmt.Errorf("%s: response over %d bytes", path, limit)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return "", err
	}
	return resp.Header.Get("ETag"), nil
}

// Snapshot fetches all tokens (about 300 KB compressed).
func (c *Client) Snapshot(ctx context.Context, etag string) (*Snapshot, string, error) {
	var s Snapshot
	tag, err := c.get(ctx, "/api/market/tokens", etag, snapshotCap, &s)
	if err != nil {
		return nil, tag, err
	}
	return &s, tag, nil
}

// Stats fetches the collection stats.
func (c *Client) Stats(ctx context.Context, etag string) (*Stats, string, error) {
	var s Stats
	tag, err := c.get(ctx, "/api/market/stats", etag, bodyCap, &s)
	if err != nil {
		return nil, tag, err
	}
	return &s, tag, nil
}

// Rate fetches the ZEC/USD rate.
func (c *Client) Rate(ctx context.Context, etag string) (*Rate, string, error) {
	var r Rate
	tag, err := c.get(ctx, "/api/market/rate", etag, bodyCap, &r)
	if err != nil {
		return nil, tag, err
	}
	return &r, tag, nil
}

// Events fetches the latest market events, newest first.
func (c *Client) Events(ctx context.Context, etag string) ([]Event, string, error) {
	var e struct {
		Events []Event `json:"events"`
	}
	tag, err := c.get(ctx, "/api/market/events", etag, bodyCap, &e)
	if err != nil {
		return nil, tag, err
	}
	return e.Events, tag, nil
}

// Sales fetches the latest sales, newest first.
func (c *Client) Sales(ctx context.Context) ([]Sale, error) {
	var s struct {
		Sales []Sale `json:"sales"`
	}
	if _, err := c.get(ctx, "/api/market/sales", "", bodyCap, &s); err != nil {
		return nil, err
	}
	return s.Sales, nil
}

// History fetches one token's provenance, oldest first. This request tells the server which
// token the user is interested in, so the app only makes it on an explicit key press.
func (c *Client) History(ctx context.Context, id int) ([]HistoryEntry, error) {
	var r struct {
		History []HistoryEntry `json:"history"`
	}
	if _, err := c.get(ctx, "/api/snarks/"+strconv.Itoa(id), "", bodyCap, &r); err != nil {
		return nil, err
	}
	return r.History, nil
}
