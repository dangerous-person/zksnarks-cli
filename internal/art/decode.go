package art

import (
	"fmt"
	"image"
	"image/draw"
	"slices"
)

const (
	sub      = 4           // texture dots sit inside the cells of a global 4 px grid
	subs     = Block / sub // subpixels per art pixel side
	dotLevel = 12          // min sum of |channel diff| for a dot to count as visible
)

// Stats reports judgement calls made while decoding one token.
type Stats struct {
	Ties       int // art pixels whose two most common colours cover the same area
	MergedDots int // dot colours mapped to the nearest palette entry because the palette was full
}

// FromImage decodes a 520x520 source image into Art.
func FromImage(img image.Image) (Art, Stats, error) {
	if b := img.Bounds(); b.Dx() != Size || b.Dy() != Size {
		return Art{}, Stats{}, fmt.Errorf("art: image is %dx%d, want %dx%d", b.Dx(), b.Dy(), Size, Size)
	}
	src := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	at := func(x, y int) RGB {
		i := y*src.Stride + x*4
		return RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]}
	}
	var st Stats
	var base [Cells]RGB
	for by := 0; by < Grid; by++ {
		for bx := 0; bx < Grid; bx++ {
			c, tie := mode(at, bx, by)
			base[by*Grid+bx] = c
			if tie {
				st.Ties++
			}
		}
	}
	var tex [Cells]uint8
	var dots [Cells]RGB
	for i := range base {
		tex[i], dots[i] = texel(at, i%Grid, i/Grid, base[i])
	}
	a, merged, err := pack(base, tex, dots)
	st.MergedDots = merged
	return a, st, err
}

// mode returns the most common colour of an art pixel's 20x20 block. When two colours cover
// the same area (an edge through the middle of the block), the colour at the block centre
// wins, so the result does not depend on map order.
func mode(at func(x, y int) RGB, bx, by int) (RGB, bool) {
	counts := map[RGB]int{}
	for y := by * Block; y < (by+1)*Block; y++ {
		for x := bx * Block; x < (bx+1)*Block; x++ {
			counts[at(x, y)]++
		}
	}
	best := 0
	for _, n := range counts {
		best = max(best, n)
	}
	var tied []RGB
	for c, n := range counts {
		if n == best {
			tied = append(tied, c)
		}
	}
	if len(tied) == 1 {
		return tied[0], false
	}
	centre := at(bx*Block+Block/2, by*Block+Block/2)
	pick := tied[0]
	for _, c := range tied {
		if c == centre {
			return c, true
		}
		if pack24(c) < pack24(pick) {
			pick = c
		}
	}
	return pick, true
}

func pack24(c RGB) int { return int(c[0])<<16 | int(c[1])<<8 | int(c[2]) }

func diff(a, b RGB) int {
	return absDiff(a[0], b[0]) + absDiff(a[1], b[1]) + absDiff(a[2], b[2])
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

type candidate struct {
	ok       bool
	strength int // diff of the strongest pixel
	col      RGB // mean colour of the dot core
}

// texel classifies the texture of one art pixel. Each of its 5x5 subpixels of 4x4 px may hold
// one dot. The lattice comes in three kinds: a global period-8 lattice (sparse on both axes),
// a per-pixel 2x2 lattice (sparse) and a period-4 lattice of 1 px dots (dense).
func texel(at func(x, y int) RGB, bx, by int, bc RGB) (uint8, RGB) {
	var cand [subs][subs]candidate
	strongest := 0
	for j := 0; j < subs; j++ {
		for i := 0; i < subs; i++ {
			x0, y0 := bx*Block+i*sub, by*Block+j*sub
			var diffs [sub * sub]int
			maxd := 0
			for k := range diffs {
				diffs[k] = diff(at(x0+k%sub, y0+k/sub), bc)
				maxd = max(maxd, diffs[k])
			}
			// A dot is a small cluster (at most 3x3 px); a whole shifted 4x4 is shading, not
			// texture. Its size is counted at half strength, so the dot's faint shadow and
			// stray dither specks in the same 4x4 do not make it look like shading.
			core := 0
			for _, d := range diffs {
				if d*2 >= maxd {
					core++
				}
			}
			if maxd < dotLevel || core > 9 {
				continue
			}
			var sum [3]int
			n := 0
			for k, d := range diffs {
				if d*4 >= maxd*3 { // dot core only, so the colour is not washed out by its edge
					c := at(x0+k%sub, y0+k/sub)
					sum[0], sum[1], sum[2] = sum[0]+int(c[0]), sum[1]+int(c[1]), sum[2]+int(c[2])
					n++
				}
			}
			cand[j][i] = candidate{true, maxd, RGB{uint8(sum[0] / n), uint8(sum[1] / n), uint8(sum[2] / n)}}
			strongest = max(strongest, maxd)
		}
	}
	// Some pixels carry a dithered shade (a rim colour bleeding into its neighbour) on top of
	// the lattice, e.g. #7724 at (18,6). Its specks pass dotLevel but are ten times fainter
	// than the lattice dots, so only dots at least half as strong as the strongest one count.
	var colDots, rowDots [subs]int
	var sum [3]int
	n := 0
	for j := 0; j < subs; j++ {
		for i := 0; i < subs; i++ {
			c := cand[j][i]
			if !c.ok || c.strength*2 < strongest {
				continue
			}
			colDots[i]++
			rowDots[j]++
			sum[0], sum[1], sum[2] = sum[0]+int(c.col[0]), sum[1]+int(c.col[1]), sum[2]+int(c.col[2])
			n++
		}
	}
	// A sparse lattice never has dots in two adjacent columns (rows), a dense one always does;
	// requiring 3 of 4 adjacent pairs keeps stray dots from flipping the call.
	cols, rows, colPairs, rowPairs := 0, 0, 0, 0
	for k := 0; k < subs; k++ {
		if colDots[k] >= 2 {
			cols++
			if k > 0 && colDots[k-1] >= 2 {
				colPairs++
			}
		}
		if rowDots[k] >= 2 {
			rows++
			if k > 0 && rowDots[k-1] >= 2 {
				rowPairs++
			}
		}
	}
	if n < 3 || cols == 0 || rows == 0 {
		return 0, RGB{}
	}
	flags := Dotted
	if colPairs >= 3 {
		flags |= DenseX
	}
	if rowPairs >= 3 {
		flags |= DenseY
	}
	return flags, RGB{uint8(sum[0] / n), uint8(sum[1] / n), uint8(sum[2] / n)}
}

// pack builds the palette: base colours first, then dot colours. Should the palette overflow
// 256 entries (the largest seen is 222), extra dot colours map to the nearest entry.
func pack(base [Cells]RGB, tex [Cells]uint8, dots [Cells]RGB) (Art, int, error) {
	a := Art{Pix: make([]uint8, Cells), Tex: make([]uint8, Cells), Dot: make([]uint8, Cells)}
	index := map[RGB]int{}
	add := func(c RGB) (int, bool) {
		if i, ok := index[c]; ok {
			return i, true
		}
		if len(a.Palette) == 256 {
			return 0, false
		}
		index[c] = len(a.Palette)
		a.Palette = append(a.Palette, c)
		return index[c], true
	}
	for i, c := range base {
		p, ok := add(c)
		if !ok {
			return Art{}, 0, fmt.Errorf("art: more than 256 base colours")
		}
		a.Pix[i] = uint8(p)
	}
	merged := 0
	for i, c := range dots {
		a.Tex[i] = tex[i]
		if tex[i]&Dotted == 0 {
			continue
		}
		p, ok := add(c)
		if !ok {
			p = nearest(a.Palette, c)
			merged++
		}
		a.Dot[i] = uint8(p)
	}
	return a, merged, nil
}

func nearest(pal []RGB, c RGB) int {
	best, bestD := 0, 1<<30
	for i, p := range pal {
		dr, dg, db := int(p[0])-int(c[0]), int(p[1])-int(c[1]), int(p[2])-int(c[2])
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// FromGrid reads a 26x26 pixel-art image at any size, such as the portraits on
// zksnarks.net (1024, 680 or 520 px, PNG or JPEG). Each art pixel is the per-channel median
// of the middle of its cell, so JPEG noise and uneven cell edges do not leak in; colours that
// differ only by such noise are then merged so flat areas stay flat. There is no texture.
func FromGrid(img image.Image) (Art, error) {
	b := img.Bounds()
	if b.Dx() < Grid || b.Dy() < Grid {
		return Art{}, fmt.Errorf("art: image is %dx%d, too small for a %dx%d grid", b.Dx(), b.Dy(), Grid, Grid)
	}
	var base [Cells]RGB
	for gy := 0; gy < Grid; gy++ {
		for gx := 0; gx < Grid; gx++ {
			x0, x1 := b.Min.X+gx*b.Dx()/Grid, b.Min.X+(gx+1)*b.Dx()/Grid
			y0, y1 := b.Min.Y+gy*b.Dy()/Grid, b.Min.Y+(gy+1)*b.Dy()/Grid
			qx, qy := (x1-x0)/4, (y1-y0)/4
			var ch [3][]int
			for y := y0 + qy; y < y1-qy; y++ {
				for x := x0 + qx; x < x1-qx; x++ {
					r, g, bl, _ := img.At(x, y).RGBA()
					ch[0], ch[1], ch[2] = append(ch[0], int(r>>8)), append(ch[1], int(g>>8)), append(ch[2], int(bl>>8))
				}
			}
			var c RGB
			for k := range ch {
				slices.Sort(ch[k])
				c[k] = uint8(ch[k][len(ch[k])/2])
			}
			base[gy*Grid+gx] = c
		}
	}
	var reps []RGB
	for i, c := range base {
		merged := false
		for _, r := range reps {
			if diff(c, r) <= 9 {
				base[i], merged = r, true
				break
			}
		}
		if !merged {
			reps = append(reps, c)
		}
	}
	a, _, err := pack(base, [Cells]uint8{}, [Cells]RGB{})
	return a, err
}
