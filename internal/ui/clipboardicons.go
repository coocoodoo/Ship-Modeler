package ui

import "image/color"

func DrawPixelSelectIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			poly(w, col, v2(cx+sx*size*.18, cy+sy*size*.37), v2(cx+sx*size*.37, cy+sy*size*.37), v2(cx+sx*size*.37, cy+sy*size*.18))
		}
	}
}

func DrawCopyPixelsIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	poly(w, col, v2(cx-size*.12, cy+size*.25), v2(cx-size*.37, cy+size*.25), v2(cx-size*.37, cy-size*.37), v2(cx+size*.2, cy-size*.37), v2(cx+size*.2, cy-size*.2))
	closedPoly(w, col, v2(cx-size*.12, cy-size*.12), v2(cx+size*.37, cy-size*.12), v2(cx+size*.37, cy+size*.4), v2(cx-size*.12, cy+size*.4))
}

func DrawPastePixelsIcon(cx, cy, size float64, col color.RGBA) {
	w := strokeWidth(size)
	closedPoly(w, col, v2(cx-size*.3, cy-size*.24), v2(cx+size*.3, cy-size*.24), v2(cx+size*.3, cy+size*.4), v2(cx-size*.3, cy+size*.4))
	closedPoly(w, col, v2(cx-size*.13, cy-size*.4), v2(cx+size*.13, cy-size*.4), v2(cx+size*.13, cy-size*.17), v2(cx-size*.13, cy-size*.17))
	line(v2(cx-size*.15, cy+size*.12), v2(cx+size*.15, cy+size*.12), w, col)
	line(v2(cx, cy-size*.03), v2(cx, cy+size*.27), w, col)
}
