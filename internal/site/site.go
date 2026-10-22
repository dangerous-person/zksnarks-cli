// Package site holds the content of zksnarks.net shown in the app: the manifesto, the
// luminaries and the network spotlights. cmd/bake fetches it from the site and it is embedded
// next to the atlas, so these sections work offline like the rest of the app.
package site

import (
	"compress/gzip"
	"encoding/json"
	"io"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// Origin is where the content comes from.
const Origin = "https://zksnarks.net"

// Span is a run of text; Hi marks the words the site highlights.
type Span struct {
	Text string `json:"t"`
	Hi   bool   `json:"hi,omitempty"`
}

// Manifesto is the site's own statement and the full text it links to.
type Manifesto struct {
	Command   string   `json:"command"` // what the prompt types, ./whoami
	Lead      []Span   `json:"lead"`    // the large first paragraph
	Body      string   `json:"body"`    // the paragraph under it
	Title     string   `json:"title"`   // A Cypherpunk's Manifesto
	Byline    string   `json:"byline"`
	Paras     [][]Span `json:"paras"`
	Signature string   `json:"signature"`
	Date      string   `json:"date"`
	Armor     string   `json:"armor"` // the decorative PGP signature block under the text
}

// Luminary is a figure the site honours, with a zkSNARK-style portrait.
type Luminary struct {
	Name     string  `json:"name"`
	Handle   string  `json:"handle"` // X handle without @
	Verified bool    `json:"verified,omitempty"`
	Cite     string  `json:"cite"`
	Art      art.Art `json:"art"`
}

// Spotlight is a collector from the network section and the token they hold.
type Spotlight struct {
	Name    string `json:"name"`
	Handle  string `json:"handle"`
	Snark   int    `json:"snark"` // token number
	Quote   string `json:"quote"`
	Bio     string `json:"bio"`
	Video   string `json:"video"` // path on the site
	Runtime string `json:"runtime"`
}

// Site is everything baked from zksnarks.net.
type Site struct {
	FetchedAt  string      `json:"fetchedAt"`
	Logo       []string    `json:"logo"` // the site's block-letter wordmark, one string per row
	Manifesto  Manifesto   `json:"manifesto"`
	Luminaries []Luminary  `json:"luminaries"`
	Spotlights []Spotlight `json:"spotlights"`
	Slots      int         `json:"slots"` // spotlight slots the site plans
}

// Encode writes the content as gzip-compressed JSON.
func Encode(w io.Writer, s *Site) error {
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(zw).Encode(s); err != nil {
		return err
	}
	return zw.Close()
}

// Decode reads content written by Encode.
func Decode(r io.Reader) (*Site, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	var s Site
	if err := json.NewDecoder(zr).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}
