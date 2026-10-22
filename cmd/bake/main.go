// Command bake builds the embedded data: it downloads the collection snapshot and the
// art of every token from zilkroad.com, decodes the art and writes
// internal/atlas/embedded/atlas.bin, then fetches the manifesto, luminaries and network
// spotlights from zksnarks.net into site.bin. Downloads are cached, so each art URL is
// fetched once.
//
//	go run ./cmd/bake
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

const (
	origin    = "https://zilkroad.com"
	userAgent = "zksnarks-cli-bake/0.1 (unofficial; +https://github.com/dangerous-person/zksnarks-cli)"
)

var (
	outPath  = flag.String("out", filepath.Join("internal", "atlas", "embedded", "atlas.bin"), "atlas file to write")
	cacheDir = flag.String("cache", defaultCache(), "download cache")
	jobs     = flag.Int("j", 4, "parallel downloads (keep it polite)")
	seedDir  = flag.String("seed", "", "directory of already downloaded N.png files to copy into the cache first")
	offline  = flag.Bool("offline", false, "use the cached snapshot and art only")
	siteOnly = flag.Bool("site-only", false, "only refresh the zksnarks.net content")
)

func defaultCache() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "zksnarks-bake")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".zksnarks-bake-cache"
	}
	return filepath.Join(home, ".cache", "zksnarks-bake")
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bake:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.MkdirAll(filepath.Join(*cacheDir, "art"), 0o755); err != nil {
		return err
	}
	if *siteOnly {
		return bakeSite()
	}
	snap, err := loadSnapshot()
	if err != nil {
		return err
	}
	fmt.Printf("snapshot %s at %s: %d tokens\n", snap.Version, snap.At, len(snap.Tokens))
	if *seedDir != "" {
		n, err := seed(snap)
		if err != nil {
			return err
		}
		fmt.Printf("seeded %d images from %s\n", n, *seedDir)
	}
	if err := download(snap); err != nil {
		return err
	}
	a, err := build(snap)
	if err != nil {
		return err
	}
	if err := addApparitions(a); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := atlas.Encode(&buf, a); err != nil {
		return err
	}
	if err := verify(buf.Bytes(), a); err != nil {
		return err
	}
	if err := writeAtomic(*outPath, buf.Bytes()); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %.2f MB\n", *outPath, float64(buf.Len())/1e6)
	return bakeSite()
}

// decimal accepts a JSON string, number or null.
type decimal string

func (d *decimal) UnmarshalJSON(b []byte) error {
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
	*d = decimal(b)
	return nil
}

type snapshot struct {
	At             string                    `json:"at"`
	Version        string                    `json:"version"`
	Minted         int                       `json:"minted"`
	Purged         int                       `json:"purged"`
	Supply         int                       `json:"supply"`
	TraitCatalogue map[string]map[string]int `json:"traitCatalogue"`
	Tokens         []struct {
		ID         int               `json:"id"`
		Img        string            `json:"img"`
		Traits     map[string]string `json:"traits"`
		Score      *float64          `json:"score"`
		Rank       *int              `json:"rank"`
		Listed     bool              `json:"listed"`
		Ask        decimal           `json:"ask"`
		Bid        decimal           `json:"bid"`
		Last       decimal           `json:"last"`
		ListedAt   *string           `json:"listedAt"`
		Purged     bool              `json:"purged"`
		Apparition *int              `json:"apparition"`
		Auction    bool              `json:"auction"`
	} `json:"tokens"`
}

var client = &http.Client{Timeout: 60 * time.Second}

// get fetches a URL with retries on network errors, 429 and 5xx.
func get(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
		default:
			return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
		}
	}
	return nil, lastErr
}

func loadSnapshot() (*snapshot, error) {
	path := filepath.Join(*cacheDir, "snapshot.json")
	var data []byte
	var err error
	if *offline {
		data, err = os.ReadFile(path)
	} else {
		data, err = get(origin + "/api/market/tokens")
		if err == nil {
			err = writeAtomic(path, data)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	if len(s.Tokens) == 0 {
		return nil, errors.New("snapshot: no tokens")
	}
	sort.Slice(s.Tokens, func(i, j int) bool { return s.Tokens[i].ID < s.Tokens[j].ID })
	return &s, nil
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9]+`)

// cachePath maps an art path such as /api/art/9697?ap=23 to a file name.
func cachePath(img string) string {
	name := strings.Trim(unsafeChars.ReplaceAllString(strings.TrimPrefix(img, "/api/art/"), "_"), "_")
	return filepath.Join(*cacheDir, "art", name+".png")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// seed copies N.png files from a previous download, only for tokens whose art path is the
// plain /api/art/N (apparitions have their own art).
func seed(s *snapshot) (int, error) {
	n := 0
	for _, t := range s.Tokens {
		if t.Img != fmt.Sprintf("/api/art/%d", t.ID) || exists(cachePath(t.Img)) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(*seedDir, strconv.Itoa(t.ID)+".png"))
		if err != nil {
			continue
		}
		if err := writeAtomic(cachePath(t.Img), data); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func download(s *snapshot) error {
	var todo []string
	for _, t := range s.Tokens {
		if !exists(cachePath(t.Img)) {
			todo = append(todo, t.Img)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	if *offline {
		return fmt.Errorf("%d images are not cached (run without -offline)", len(todo))
	}
	fmt.Printf("downloading %d images with %d connections\n", len(todo), *jobs)
	start := time.Now()
	var done, failed atomic.Int64
	work := make(chan string)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	for range *jobs {
		wg.Go(func() {
			for img := range work {
				data, err := get(origin + img)
				if err == nil {
					err = writeAtomic(cachePath(img), data)
				}
				if err != nil {
					failed.Add(1)
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				if n := done.Add(1); n%250 == 0 || int(n) == len(todo) {
					rate := float64(n) / time.Since(start).Seconds()
					fmt.Printf("  %d/%d  %.1f/s  eta %s\n", n, len(todo), rate, (time.Duration(float64(len(todo)-int(n))/rate) * time.Second).Round(time.Second))
				}
			}
		})
	}
	for _, img := range todo {
		work <- img
	}
	close(work)
	wg.Wait()
	if failed.Load() > 0 {
		return fmt.Errorf("%d downloads failed (rerun to retry), first: %w", failed.Load(), firstErr)
	}
	return nil
}

// build decodes every cached image in parallel and assembles the atlas.
func build(s *snapshot) (*atlas.Atlas, error) {
	a := &atlas.Atlas{
		Meta: atlas.Meta{
			Format:     atlas.Format,
			Snapshot:   s.Version,
			SnapshotAt: s.At,
			Minted:     s.Minted,
			Supply:     s.Supply,
			Purged:     s.Purged,
			Counts:     s.TraitCatalogue,
		},
		Tokens: make([]atlas.Token, len(s.Tokens)),
	}
	var ties, merged, textured, maxPal atomic.Int64
	var wg sync.WaitGroup
	errs := make([]error, len(s.Tokens))
	next := atomic.Int64{}
	for range runtime.NumCPU() {
		wg.Go(func() {
			for {
				i := int(next.Add(1) - 1)
				if i >= len(s.Tokens) {
					return
				}
				t := s.Tokens[i]
				f, err := os.Open(cachePath(t.Img))
				if err != nil {
					errs[i] = err
					continue
				}
				img, err := png.Decode(f)
				f.Close()
				if err != nil {
					errs[i] = fmt.Errorf("token %d: %w", t.ID, err)
					continue
				}
				ar, st, err := art.FromImage(img)
				if err != nil {
					errs[i] = fmt.Errorf("token %d: %w", t.ID, err)
					continue
				}
				ties.Add(int64(st.Ties))
				merged.Add(int64(st.MergedDots))
				for _, f := range ar.Tex {
					if f&art.Dotted != 0 {
						textured.Add(1)
						break
					}
				}
				for {
					cur := maxPal.Load()
					if int64(len(ar.Palette)) <= cur || maxPal.CompareAndSwap(cur, int64(len(ar.Palette))) {
						break
					}
				}
				tok := atlas.Token{
					ID:      t.ID,
					Img:     t.Img,
					Listed:  t.Listed,
					Ask:     string(t.Ask),
					Bid:     string(t.Bid),
					Last:    string(t.Last),
					Purged:  t.Purged,
					Auction: t.Auction,
					Art:     ar,
				}
				for k, name := range atlas.TraitNames {
					tok.Traits[k] = t.Traits[name]
				}
				if t.Rank != nil {
					tok.Rank = *t.Rank
				}
				if t.Score != nil {
					tok.Score = *t.Score
				}
				if t.ListedAt != nil {
					tok.ListedAt = *t.ListedAt
				}
				if t.Apparition != nil {
					tok.Apparition = *t.Apparition
				}
				a.Tokens[i] = tok
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	fmt.Printf("decoded %d tokens: %d textured, largest palette %d, %d tied pixels, %d merged dot colours\n",
		len(a.Tokens), textured.Load(), maxPal.Load(), ties.Load(), merged.Load())
	return a, nil
}

// addApparitions records the apparition slots and which slot every purged token unveiled.
// Both endpoints are about the whole collection.
func addApparitions(a *atlas.Atlas) error {
	var slots struct {
		Slots []struct {
			Apparition *struct {
				Slot       int    `json:"slot"`
				Token      int    `json:"token"`
				Name       string `json:"name"`
				UnlockedAt string `json:"unlockedAt"`
			} `json:"apparition"`
		} `json:"slots"`
	}
	var purged struct {
		Purged []struct {
			Token int `json:"token"`
			Slot  int `json:"slot"`
		} `json:"purged"`
	}
	for _, f := range []struct {
		path string
		v    any
	}{{"/api/market/apparitions", &slots}, {"/api/market/apparitions/purged", &purged}} {
		cache := filepath.Join(*cacheDir, strings.ReplaceAll(strings.TrimPrefix(f.path, "/api/market/"), "/", "_")+".json")
		var data []byte
		var err error
		if *offline {
			data, err = os.ReadFile(cache)
		} else if data, err = get(origin + f.path); err == nil {
			err = writeAtomic(cache, data)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", f.path, err)
		}
		if err := json.Unmarshal(data, f.v); err != nil {
			return fmt.Errorf("%s: %w", f.path, err)
		}
	}
	for _, s := range slots.Slots {
		if s.Apparition != nil {
			a.Meta.Apparitions = append(a.Meta.Apparitions, atlas.Apparition{Slot: s.Apparition.Slot, Token: s.Apparition.Token, Name: s.Apparition.Name, UnlockedAt: s.Apparition.UnlockedAt})
		}
	}
	byID := map[int]int{}
	for i, t := range a.Tokens {
		byID[t.ID] = i
	}
	for _, p := range purged.Purged {
		if i, ok := byID[p.Token]; ok {
			a.Tokens[i].PurgedFor = p.Slot
		}
	}
	fmt.Printf("%d apparitions, %d purged tokens mapped to them\n", len(a.Meta.Apparitions), len(purged.Purged))
	return nil
}

// verify decodes the encoded atlas and checks that it matches what was built.
func verify(data []byte, want *atlas.Atlas) error {
	got, err := atlas.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if len(got.Tokens) != len(want.Tokens) {
		return fmt.Errorf("verify: %d tokens, want %d", len(got.Tokens), len(want.Tokens))
	}
	for i, w := range want.Tokens {
		g := got.Tokens[i]
		if g.ID != w.ID || g.Traits != w.Traits || g.Img != w.Img ||
			!bytes.Equal(g.Art.Pix, w.Art.Pix) || !bytes.Equal(g.Art.Tex, w.Art.Tex) || !bytes.Equal(g.Art.Dot, w.Art.Dot) ||
			len(g.Art.Palette) != len(w.Art.Palette) {
			return fmt.Errorf("verify: token %d does not round-trip", w.ID)
		}
		for j := range w.Art.Palette {
			if g.Art.Palette[j] != w.Art.Palette[j] {
				return fmt.Errorf("verify: token %d palette does not round-trip", w.ID)
			}
		}
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
