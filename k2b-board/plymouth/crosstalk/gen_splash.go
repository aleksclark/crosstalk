//go:build ignore

// Generates a placeholder splash.png for the Plymouth boot theme.
// Run: go run gen_splash.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	const (
		w = 480
		h = 200
	)

	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Dark background
	bg := color.RGBA{18, 18, 26, 255}
	for y := range h {
		for x := range w {
			img.Set(x, y, bg)
		}
	}

	// Draw "CROSSTALK" in blocky pixel font (8x scale)
	// Each letter is a 5x7 grid of pixels scaled up.
	letters := map[byte][]string{
		'C': {
			" ### ",
			"#    ",
			"#    ",
			"#    ",
			"#    ",
			"#    ",
			" ### ",
		},
		'R': {
			"#### ",
			"#   #",
			"#   #",
			"#### ",
			"# #  ",
			"#  # ",
			"#   #",
		},
		'O': {
			" ### ",
			"#   #",
			"#   #",
			"#   #",
			"#   #",
			"#   #",
			" ### ",
		},
		'S': {
			" ### ",
			"#    ",
			"#    ",
			" ### ",
			"    #",
			"    #",
			"### ",
		},
		'T': {
			"#####",
			"  #  ",
			"  #  ",
			"  #  ",
			"  #  ",
			"  #  ",
			"  #  ",
		},
		'A': {
			" ### ",
			"#   #",
			"#   #",
			"#####",
			"#   #",
			"#   #",
			"#   #",
		},
		'L': {
			"#    ",
			"#    ",
			"#    ",
			"#    ",
			"#    ",
			"#    ",
			"#####",
		},
		'K': {
			"#   #",
			"#  # ",
			"# #  ",
			"##   ",
			"# #  ",
			"#  # ",
			"#   #",
		},
	}

	text := "CROSSTALK"
	scale := 4
	letterW := 5*scale + scale // 5 pixels + 1 pixel gap, scaled
	totalW := len(text) * letterW
	startX := (w - totalW) / 2
	startY := (h - 7*scale) / 2

	fg := color.RGBA{160, 180, 220, 255}

	for i, ch := range text {
		glyph, ok := letters[byte(ch)]
		if !ok {
			continue
		}
		ox := startX + i*letterW
		for row, line := range glyph {
			for col, c := range line {
				if c == '#' {
					for dy := range scale {
						for dx := range scale {
							img.Set(ox+col*scale+dx, startY+row*scale+dy, fg)
						}
					}
				}
			}
		}
	}

	f, err := os.Create("splash.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
