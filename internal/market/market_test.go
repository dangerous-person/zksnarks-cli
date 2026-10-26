package market

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type toServer struct {
	host string
	rt   http.RoundTripper
}

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = "http", t.host
	return t.rt.RoundTrip(r)
}

func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	c := New(false)
	c.HTTP.Transport = toServer{strings.TrimPrefix(ts.URL, "http://"), ts.Client().Transport}
	return c
}

func TestRedirects(t *testing.T) {
	cases := []struct {
		name, to string
		hops     int
		err      string
	}{
		{"other host", "https://zilkroad.example/hit", 1, "refused a redirect to https://zilkroad.example"},
		{"plain http", "http://zilkroad.com/hit", 1, "refused a redirect to http://zilkroad.com"},
		{"other port", "https://zilkroad.com:8443/hit", 1, "refused a redirect to https://zilkroad.com:8443"},
		{"three hops", "https://zilkroad.com/hit", 3, ""},
		{"relative", "/hit", 3, ""},
		{"four hops", "https://zilkroad.com/hit", 4, "stopped after 3 redirects"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var hits atomic.Int32
			cl := serve(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/hit" {
					hits.Add(1)
					w.Write([]byte(`{"floorZec":"0.5","sales24h":3}`))
					return
				}
				n, _ := strconv.Atoi(r.URL.Query().Get("n"))
				if n+1 < c.hops {
					http.Redirect(w, r, "/api/market/stats?n="+strconv.Itoa(n+1), http.StatusFound)
					return
				}
				http.Redirect(w, r, c.to, http.StatusFound)
			})
			s, _, err := cl.Stats(context.Background(), "")
			if c.err == "" {
				if err != nil || s.Floor != "0.5" || s.Sales24h != 3 {
					t.Fatalf("got %+v, %v; want the stats", s, err)
				}
				if hits.Load() != 1 {
					t.Fatalf("target hit %d times, want 1", hits.Load())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("got %+v, %v; want %q", s, err, c.err)
			}
			if hits.Load() != 0 {
				t.Fatalf("target hit %d times, want 0", hits.Load())
			}
		})
	}
}

func TestBodyCap(t *testing.T) {
	body := func(n int) []byte {
		b := []byte(`{"floorZec":"0.5"}`)
		return append(b, []byte(strings.Repeat(" ", n-len(b)))...)
	}
	cases := []struct {
		path string
		size int
		ok   bool
	}{
		{"/api/market/stats", bodyCap, true},
		{"/api/market/stats", bodyCap + 1, false},
		{"/api/market/rate", bodyCap + 1, false},
		{"/api/market/events", bodyCap + 1, false},
		{"/api/snarks/7", bodyCap + 1, false},
		{"/api/market/tokens", bodyCap + 1, true},
		{"/api/market/tokens", snapshotCap, true},
		{"/api/market/tokens", snapshotCap + 1, false},
	}
	for _, c := range cases {
		cl := serve(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != c.path {
				http.NotFound(w, r)
				return
			}
			w.Write(body(c.size))
		})
		var err error
		switch c.path {
		case "/api/market/stats":
			_, _, err = cl.Stats(context.Background(), "")
		case "/api/market/rate":
			_, _, err = cl.Rate(context.Background(), "")
		case "/api/market/events":
			_, _, err = cl.Events(context.Background(), "")
		case "/api/market/tokens":
			_, _, err = cl.Snapshot(context.Background(), "")
		default:
			_, err = cl.History(context.Background(), 7)
		}
		if c.ok != (err == nil) {
			t.Errorf("%s with %d bytes: err %v, want ok %v", c.path, c.size, err, c.ok)
		}
		if !c.ok && err != nil && !strings.Contains(err.Error(), "response over") {
			t.Errorf("%s with %d bytes: err %v, want the size error", c.path, c.size, err)
		}
	}
}

func TestETag(t *testing.T) {
	var seen []string
	cl := serve(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte(`{"zecUsd":1190.85}`))
	})
	r, tag, err := cl.Rate(context.Background(), "")
	if err != nil || r.ZecUsd != 1190.85 || tag != `"v1"` {
		t.Fatalf("first: %+v, %q, %v", r, tag, err)
	}
	r, tag, err = cl.Rate(context.Background(), tag)
	if !errors.Is(err, ErrNotModified) || r != nil || tag != `"v1"` {
		t.Fatalf("second: %+v, %q, %v; want not modified", r, tag, err)
	}
	if len(seen) != 2 || seen[0] != "" || seen[1] != `"v1"` {
		t.Fatalf("If-None-Match sent: %q", seen)
	}
}

func TestNotModifiedWithoutETag(t *testing.T) {
	cl := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	if _, _, err := cl.Stats(context.Background(), ""); err == nil || errors.Is(err, ErrNotModified) {
		t.Fatalf("got %v, want an HTTP error", err)
	}
}

func TestDecimalRejectsJunk(t *testing.T) {
	cases := map[string]Decimal{
		`"2.45"`: "2.45", `3`: "3", `"10"`: "10", `null`: "",
		`"NaN"`: "", `"0x1p-2"`: "", `"1e3"`: "", `"1_000"`: "", `".5"`: "", `"5."`: "", `"-1"`: "",
		`"` + strings.Repeat("9", 40) + `"`: "",
	}
	for in, want := range cases {
		var d Decimal
		if err := json.Unmarshal([]byte(in), &d); err != nil || d != want {
			t.Errorf("%s: %q, %v (want %q)", in, d, err, want)
		}
	}
}

func TestHistoryNameIsPrintable(t *testing.T) {
	if got := (HistoryEntry{Type: "odd\x1b]52;c;x\x07_type é"}).Name(); got != "odd]52;c;x type" {
		t.Errorf("name %q", got)
	}
}
