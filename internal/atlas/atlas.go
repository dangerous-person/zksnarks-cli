// Package atlas holds the whole collection: metadata and artwork for all 10,000 tokens,
// baked by cmd/bake and embedded in the binary. It is the only source of art, so the
// app never makes a request about an individual token to draw it.
package atlas

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// Format is the version of the binary layout.
const Format = 1

var magic = [4]byte{'S', 'N', 'K', 'T'}

// TraitNames lists the traits in display order; Token.Traits follows it.
var TraitNames = [6]string{"Lineage", "Skin", "Eyes", "Head", "Accessory", "Background"}

// Token is one zkSNARK as of the snapshot the atlas was baked from. Market fields are only a
// fallback for when the live market data cannot be fetched.
type Token struct {
	ID         int       `json:"id"`
	Img        string    `json:"img"` // art path on zilkroad.com, e.g. /api/art/9697?ap=23
	Traits     [6]string `json:"traits"`
	Rank       int       `json:"rank,omitempty"` // 0 for apparitions, which are unranked
	Score      float64   `json:"score,omitempty"`
	Purged     bool      `json:"purged,omitempty"`
	Apparition int       `json:"apparition,omitempty"` // apparition number, 0 if none
	Listed     bool      `json:"listed,omitempty"`
	Ask        string    `json:"ask,omitempty"`  // ZEC, as the API writes it
	Bid        string    `json:"bid,omitempty"`  // best open bid, ZEC
	Last       string    `json:"last,omitempty"` // last sale, ZEC
	ListedAt   string    `json:"listedAt,omitempty"`
	Auction    bool      `json:"auction,omitempty"`
	PurgedFor  int       `json:"purgedFor,omitempty"` // apparition slot this token was purged to unveil

	Art art.Art `json:"-"`
}

// Apparition is one of the apparition slots. Purging 26 tokens unveils one.
type Apparition struct {
	Slot       int    `json:"slot"`
	Token      int    `json:"token"`
	Name       string `json:"name"`
	UnlockedAt string `json:"unlockedAt"`
}

// Meta describes the snapshot the atlas was baked from.
type Meta struct {
	Format      int                       `json:"format"`
	Snapshot    string                    `json:"snapshot"`   // the API's collection version
	SnapshotAt  string                    `json:"snapshotAt"` // when the API took the snapshot
	Minted      int                       `json:"minted"`
	Supply      int                       `json:"supply"` // minted minus purged
	Purged      int                       `json:"purged"`
	Counts      map[string]map[string]int `json:"counts"` // live tokens per trait value
	Apparitions []Apparition              `json:"apparitions"`
}

// ApparitionBySlot returns the apparition in a slot.
func (m *Meta) ApparitionBySlot(slot int) (Apparition, bool) {
	for _, a := range m.Apparitions {
		if a.Slot == slot {
			return a, true
		}
	}
	return Apparition{}, false
}

// Atlas is the decoded collection, tokens in ID order.
type Atlas struct {
	Meta   Meta
	Tokens []Token
}

type file struct {
	Meta   Meta    `json:"meta"`
	Tokens []Token `json:"tokens"`
}

// Encode writes the atlas: a gzip stream of the header, the metadata as JSON and one record
// per token (palette size - 1, palette, then pixel, texture and dot planes of 676 bytes).
func Encode(w io.Writer, a *Atlas) error {
	meta, err := json.Marshal(file{a.Meta, a.Tokens})
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(zw)
	bw.Write(magic[:])
	bw.WriteByte(Format)
	binary.Write(bw, binary.LittleEndian, uint32(len(meta)))
	bw.Write(meta)
	for _, t := range a.Tokens {
		p := t.Art.Palette
		if len(p) == 0 || len(p) > 256 {
			return fmt.Errorf("atlas: token %d has %d palette entries", t.ID, len(p))
		}
		bw.WriteByte(byte(len(p) - 1))
		for _, c := range p {
			bw.Write(c[:])
		}
		bw.Write(t.Art.Pix)
		bw.Write(t.Art.Tex)
		bw.Write(t.Art.Dot)
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// Decode reads an atlas written by Encode. Art planes alias one shared buffer.
func Decode(r io.Reader) (*Atlas, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if len(raw) < 9 || !bytes.Equal(raw[:4], magic[:]) {
		return nil, errors.New("atlas: not an atlas file")
	}
	if raw[4] != Format {
		return nil, fmt.Errorf("atlas: format %d, want %d (re-run cmd/bake)", raw[4], Format)
	}
	n := int(binary.LittleEndian.Uint32(raw[5:9]))
	if 9+n > len(raw) {
		return nil, errors.New("atlas: truncated metadata")
	}
	var f file
	if err := json.Unmarshal(raw[9:9+n], &f); err != nil {
		return nil, fmt.Errorf("atlas: metadata: %w", err)
	}
	rest := raw[9+n:]
	for i := range f.Tokens {
		if len(rest) < 1 {
			return nil, errors.New("atlas: truncated art")
		}
		np := int(rest[0]) + 1
		need := 1 + np*3 + 3*art.Cells
		if len(rest) < need {
			return nil, errors.New("atlas: truncated art")
		}
		pal := make([]art.RGB, np)
		for j := range pal {
			copy(pal[j][:], rest[1+j*3:])
		}
		planes := rest[1+np*3 : need : need]
		f.Tokens[i].Art = art.Art{
			Palette: pal,
			Pix:     planes[:art.Cells:art.Cells],
			Tex:     planes[art.Cells : 2*art.Cells : 2*art.Cells],
			Dot:     planes[2*art.Cells:],
		}
		rest = rest[need:]
	}
	if len(rest) != 0 {
		return nil, errors.New("atlas: trailing data")
	}
	return &Atlas{Meta: f.Meta, Tokens: f.Tokens}, nil
}
