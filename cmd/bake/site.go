package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

var siteOut = filepath.Join("internal", "atlas", "embedded", "site.bin")

// bakeSite fetches the manifesto, luminaries and network spotlights from zksnarks.net. The
// site is a single page whose content sits in its scripts, so this reads those literals.
func bakeSite() error {
	index, err := siteGet("/", "index.html")
	if err != nil {
		return err
	}
	page := string(index)
	s := &site.Site{}

	if s.Manifesto, err = manifesto(page); err != nil {
		return fmt.Errorf("manifesto: %w", err)
	}
	if s.Luminaries, err = luminaries(page); err != nil {
		return fmt.Errorf("luminaries: %w", err)
	}
	if s.Spotlights, s.Slots, err = spotlights(); err != nil {
		return fmt.Errorf("network: %w", err)
	}
	var buf bytes.Buffer
	if err := site.Encode(&buf, s); err != nil {
		return err
	}
	if err := writeAtomic(siteOut, buf.Bytes()); err != nil {
		return err
	}
	fmt.Printf("site: manifesto %d paragraphs, %d luminaries, %d spotlights; wrote %s: %.1f KB\n",
		len(s.Manifesto.Paras), len(s.Luminaries), len(s.Spotlights), siteOut, float64(buf.Len())/1e3)
	return nil
}

// siteGet fetches a path from zksnarks.net through the cache.
func siteGet(path, name string) ([]byte, error) {
	cache := filepath.Join(*cacheDir, "site", name)
	if *offline {
		return os.ReadFile(cache)
	}
	data, err := get(site.Origin + path)
	if err != nil {
		return nil, err
	}
	return data, writeAtomic(cache, data)
}

// jsString reads the JavaScript string literal that starts at s[i] and returns it and the
// index after its closing quote.
func jsString(s string, i int) (string, int, error) {
	q := s[i]
	if q != '"' && q != '\'' {
		return "", 0, errors.New("not a string literal")
	}
	var b strings.Builder
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		switch {
		case c == q:
			return b.String(), j + 1, nil
		case c == '\\' && j+1 < len(s):
			j++
			switch s[j] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if j+4 < len(s) {
					if r, err := strconv.ParseUint(s[j+1:j+5], 16, 32); err == nil {
						b.WriteRune(rune(r))
						j += 4
						continue
					}
				}
				b.WriteByte('u')
			default:
				b.WriteByte(s[j])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, errors.New("unterminated string literal")
}

// stringAfter returns the string literal that follows the first occurrence of marker.
func stringAfter(s, marker string) (string, error) {
	i := strings.Index(s, marker)
	if i < 0 {
		return "", fmt.Errorf("%q not found", marker)
	}
	j := i + len(marker)
	for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\t') {
		j++
	}
	v, _, err := jsString(s, j)
	return v, err
}

// closing returns the index just past the bracket that closes the one at s[i], skipping
// string literals.
func closing(s string, i int) (int, error) {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '"', '\'', '`':
			if s[j] == '`' {
				k := strings.IndexByte(s[j+1:], '`')
				if k < 0 {
					return 0, errors.New("unterminated template")
				}
				j += k + 1
				continue
			}
			_, end, err := jsString(s, j)
			if err != nil {
				return 0, err
			}
			j = end - 1
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return j + 1, nil
			}
		}
	}
	return 0, errors.New("unbalanced brackets")
}

// literalAfter returns the array or object literal that follows marker.
func literalAfter(s, marker string) (string, error) {
	i := strings.Index(s, marker)
	if i < 0 {
		return "", fmt.Errorf("%q not found", marker)
	}
	j := i + len(marker)
	for j < len(s) && s[j] != '[' && s[j] != '{' {
		j++
	}
	end, err := closing(s, j)
	if err != nil {
		return "", err
	}
	return s[j:end], nil
}

// jsToJSON turns a JavaScript data literal (bare keys, either quote, trailing commas) into
// JSON. Expressions other than literals are not supported.
func jsToJSON(s string) (string, error) {
	var b strings.Builder
	last := byte(0) // last significant byte written
	write := func(str string) {
		b.WriteString(str)
		if t := strings.TrimRight(str, " \n\t\r"); t != "" {
			last = t[len(t)-1]
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			v, end, err := jsString(s, i)
			if err != nil {
				return "", err
			}
			q, _ := json.Marshal(v)
			write(string(q))
			i = end
		case c == ']' || c == '}':
			if last == ',' { // drop a trailing comma
				out := strings.TrimRight(b.String(), " \n\t\r")
				b.Reset()
				b.WriteString(out[:len(out)-1])
			}
			write(string(c))
			i++
		case c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(s) && (s[j] == '_' || s[j] == '$' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			id := s[i:j]
			k := j
			for k < len(s) && (s[k] == ' ' || s[k] == '\n' || s[k] == '\t' || s[k] == '\r') {
				k++
			}
			switch {
			case k < len(s) && s[k] == ':':
				write(`"` + id + `"`)
			case id == "true" || id == "false" || id == "null":
				write(id)
			default:
				return "", fmt.Errorf("unsupported expression %q", id)
			}
			i = j
		default:
			write(string(c))
			i++
		}
	}
	return b.String(), nil
}

var (
	tagRe   = regexp.MustCompile(`<[^>]+>`)
	spaceRe = regexp.MustCompile(`\s+`)
	hiRe    = regexp.MustCompile(`<span class="(?:g|hl)">(.*?)</span>`)
)

// text strips tags, decodes entities and collapses white space.
func text(h string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(h, "")), " "))
}

// spans splits HTML into plain and highlighted runs.
func spans(h string) []site.Span {
	var out []site.Span
	add := func(t string, hi bool) {
		t = spaceRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(t, "")), " ")
		if t != "" {
			out = append(out, site.Span{Text: t, Hi: hi})
		}
	}
	rest := h
	for {
		loc := hiRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			add(rest, false)
			break
		}
		add(rest[:loc[0]], false)
		add(rest[loc[2]:loc[3]], true)
		rest = rest[loc[1]:]
	}
	if len(out) > 0 {
		out[0].Text = strings.TrimLeft(out[0].Text, " ")
		out[len(out)-1].Text = strings.TrimRight(out[len(out)-1].Text, " ")
	}
	return out
}

var (
	paraRe  = regexp.MustCompile(`(?s)<p class="para[^"]*"[^>]*>(.*?)</p>`)
	titleRe = regexp.MustCompile(`<h2>([^<]+)</h2>" \+ '<div class="by">([^<]+)</div>`)
	whoRe   = regexp.MustCompile(`(?s)<div class="who">(.*?)</div>\s*<div class="date">(.*?)</div>`)
)

func manifesto(page string) (site.Manifesto, error) {
	var m site.Manifesto
	var err error
	lead, err := stringAfter(page, `p1.innerHTML =`)
	if err != nil {
		return m, err
	}
	m.Lead = spans(strings.ReplaceAll(lead, "zkSNARKS", "zkSNARKs"))
	if m.Body, err = stringAfter(page, `const P2 =`); err != nil {
		return m, err
	}
	t := titleRe.FindStringSubmatch(page)
	if t == nil {
		return m, errors.New("title not found")
	}
	m.Title = text(t[1])
	full, err := siteGet("/manifesto.html", "manifesto.html")
	if err != nil {
		return m, err
	}
	for _, p := range paraRe.FindAllStringSubmatch(string(full), -1) {
		m.Paras = append(m.Paras, spans(p[1]))
	}
	if len(m.Paras) == 0 {
		return m, errors.New("no paragraphs")
	}
	if w := whoRe.FindStringSubmatch(string(full)); w != nil {
		m.Signature, m.Date = text(w[1]), text(w[2])
	}
	return m, nil
}

func luminaries(page string) ([]site.Luminary, error) {
	lit, err := literalAfter(page, "const LUMINARIES =")
	if err != nil {
		return nil, err
	}
	js, err := jsToJSON(lit)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Art      string `json:"art"`
		Name     string `json:"name"`
		Handle   string `json:"handle"`
		Verified bool   `json:"verified"`
		Cite     string `json:"cite"`
	}
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return nil, err
	}
	var out []site.Luminary
	for _, r := range raw {
		data, err := siteGet("/"+r.Art, filepath.Base(r.Art))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Art, err)
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Art, err)
		}
		a, err := art.FromGrid(img)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Art, err)
		}
		out = append(out, site.Luminary{Name: r.Name, Handle: r.Handle, Verified: r.Verified, Cite: r.Cite, Art: a})
	}
	return out, nil
}

var slotsRe = regexp.MustCompile(`slots:\s*(\d+)`)

func spotlights() ([]site.Spotlight, int, error) {
	data, err := siteGet("/network/spotlights.js", "spotlights.js")
	if err != nil {
		return nil, 0, err
	}
	lit, err := literalAfter(string(data), "spotlights:")
	if err != nil {
		return nil, 0, err
	}
	js, err := jsToJSON(lit)
	if err != nil {
		return nil, 0, err
	}
	var out []site.Spotlight
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, 0, err
	}
	slots := len(out)
	if m := slotsRe.FindStringSubmatch(string(data)); m != nil {
		slots, _ = strconv.Atoi(m[1])
	}
	return out, slots, nil
}
