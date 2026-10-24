package ui

import (
	"strings"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
)

// overlay reports whether the card is open over the results.
func (m *Model) overlay() bool { return m.section == secZilkroad && m.card }

type rect struct{ x0, y0, x1, y1 int }

func (r rect) contains(x, y int) bool { return x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1 }

// drawCard layers the card over the dimmed results: a box with a drop shadow, its title
// on the top of the frame and its navigation on the bottom, both in line with the art. It
// records where it drew, so a click outside can close the card and a click on a key hint
// in it does what the key does.
func (m *Model) drawCard(cv *canvas) {
	t, ok := m.selected()
	if !ok {
		return
	}
	i := m.results[m.sel]
	top, bottom := m.headRows(), m.h-m.footRows() // body rows
	left, right := 2, m.w-2
	areaW, areaH := right-left, bottom-top
	cv.shade(0, top, m.w, bottom, 1-m.theme.Backdrop, art.RGB{})
	const padY = modalPadY
	chrome := modalChromeW
	artLines, artW := m.cardArt(i), art.CardW
	infoW := 44
	contentH := art.CardH
	if m.histFor != t.ID { // the details grow the box when they are taller than the art
		contentH = max(contentH, len(m.cardInfo(t, infoW, areaH, false, 0, 0)))
	}
	if chrome+artW+3+infoW+2 > areaW || 2+2*padY+contentH+1 > areaH {
		artLines, artW = m.thumb(i), art.ThumbW
		infoW = min(44, areaW-chrome-artW-3-2)
		contentH = min(areaH-1-2-2*padY, 28)
	}
	boxW := chrome + artW + 3 + infoW
	boxH := 2 + 2*padY + contentH
	x0 := left + (areaW-boxW)/2
	y0 := top + (areaH-boxH)/2
	ax, ay := m.modalBox(cv, rect{x0, y0, x0 + boxW, y0 + boxH}, title(t), func(w int) line { return m.cardNav(t, w) })
	for k, l := range artLines[:min(len(artLines), contentH)] {
		cv.draw(ax, ay+k, l)
	}
	m.keys = nil
	for k, l := range m.cardInfo(t, infoW, contentH, false, ax+artW+3, ay) {
		cv.draw(ax+artW+3, ay+k, l.s)
	}
}

// The modals' box: a column of panel colour on each side, the frame, then the padding.
const modalMargin, modalPadX, modalPadY = 1, 2, 1

// modalChrome is how much wider and taller than its content a modal's box is.
const modalChromeW, modalChromeH = 2*modalMargin + 2 + 2*modalPadX, 2 + 2*modalPadY

// modalBox draws a modal's box at b with a drop shadow, the title on the top of its frame
// and nav, given the room it has, on the bottom, both in line with the content. It records
// the box, so a click outside can close it, and returns where the content starts.
func (m *Model) modalBox(cv *canvas, b rect, titleText string, nav func(w int) line) (int, int) {
	m.box = b
	k := 1 - m.theme.Shadow
	cv.shade(b.x1, b.y0+1, b.x1+2, b.y1+1, k, art.RGB{}) // right edge of the shadow
	cv.shade(b.x0+2, b.y1, b.x1, b.y1+1, k, art.RGB{})   // bottom edge
	cv.fill(b.x0, b.y0, b.x1, b.y1, m.theme.ModalBg)
	fx0, fx1 := b.x0+modalMargin, b.x1-modalMargin // the frame
	bc := fg(m.theme.ModalBorder)
	cv.draw(fx0, b.y0, bc+"╔"+strings.Repeat("═", fx1-fx0-2)+"╗"+reset)
	for y := b.y0 + 1; y < b.y1-1; y++ {
		cv.draw(fx0, y, bc+"║"+reset)
		cv.draw(fx1-1, y, bc+"║"+reset)
	}
	cv.draw(fx0, b.y1-1, bc+"╚"+strings.Repeat("═", fx1-fx0-2)+"╝"+reset)
	ax, ay := fx0+1+modalPadX, b.y0+1+modalPadY
	cv.draw(ax, b.y0, chip(m.theme.ModalTitleBg, m.theme.ModalTitleFg, " "+titleText+" "))
	n := nav(fx1 - 1 - ax - 2) // a space and a dash before the corner
	cv.draw(ax-1, b.y1-1, " "+n.s+" ")
	return ax, ay
}

// cardNav is the card's navigation for the bottom of its frame; the card's other keys are
// on the card itself.
func (m *Model) cardNav(t *atlas.Token, w int) line {
	pairs := []string{"←→", "prev/next", "esc", "back"}
	if h := m.histories[t.ID]; m.histFor == t.ID && h != nil && !h.loading && h.err == nil {
		pairs = append([]string{"↑↓", "scroll"}, pairs...)
	}
	if l := m.hints(pairs...); l.w <= w {
		return l
	}
	return m.hints("←→", "next", "esc", "back")
}
