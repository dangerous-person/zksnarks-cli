// Package art turns zkSNARKs artwork into terminal cells.
//
// Every token is a 26x26 pixel-art grid, upscaled 20x to a 520x520 PNG on a pure black
// background. About half the lineages also carry a texture of small dots on a 4 px grid.
// Art keeps both in a compact palette-indexed form, and the renderers turn it into cells.
package art

const (
	Grid  = 26           // art pixels per side
	Block = 20           // source pixels per art pixel
	Size  = Grid * Block // source image side
	Cells = Grid * Grid  // art pixels per token
)

// RGB is a 24-bit colour.
type RGB [3]uint8

// Texture flags, one byte per art pixel.
const (
	Dotted uint8 = 1 << iota // the pixel carries a dot lattice
	DenseX                   // dots in every column slot instead of every other one
	DenseY                   // dots in every row slot instead of every other one
)

// Art is one token's artwork: a palette of base and dot colours, the base colour of every
// art pixel and its texture. Pix, Tex and Dot may alias a shared atlas buffer.
type Art struct {
	Palette []RGB
	Pix     []uint8 // Cells palette indices, row-major
	Tex     []uint8 // Cells texture flags
	Dot     []uint8 // Cells palette indices of the dot colour, meaningful where Dotted is set
}

// Base returns the base colour of the art pixel at x, y.
func (a Art) Base(x, y int) RGB { return a.Palette[a.Pix[y*Grid+x]] }

// Texel returns the texture flags and dot colour of the art pixel at x, y.
func (a Art) Texel(x, y int) (uint8, RGB) {
	i := y*Grid + x
	return a.Tex[i], a.Palette[a.Dot[i]]
}
