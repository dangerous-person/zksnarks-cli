// Package embedded carries the data baked by cmd/bake: the atlas of all tokens and the
// zksnarks.net content. It is separate from package atlas so that the baker builds before
// the first atlas exists.
package embedded

import (
	"bytes"
	_ "embed"
	"sync"

	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/site"
)

//go:embed atlas.bin
var raw []byte

//go:embed site.bin
var siteRaw []byte

// Size is the compressed size of the embedded atlas in bytes.
var Size = len(raw)

// Load decodes the embedded atlas once and returns it.
var Load = sync.OnceValues(func() (*atlas.Atlas, error) {
	return atlas.Decode(bytes.NewReader(raw))
})

// LoadSite decodes the embedded zksnarks.net content once and returns it.
var LoadSite = sync.OnceValues(func() (*site.Site, error) {
	return site.Decode(bytes.NewReader(siteRaw))
})
