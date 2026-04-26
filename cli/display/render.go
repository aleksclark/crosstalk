package display

import (
	"fmt"
	"image"
	"image/color"
)

const (
	screenW = 320
	screenH = 240
	pad     = 3
	lineH   = 12
)

// Render draws the full status UI into a 320×240 RGBA image.
//
// Layout (320×240 landscape):
//
//	┌─ NETWORK ──────────────────────┐  y=0..34
//	│ wlan0  192.168.0.109      [UP] │
//	└────────────────────────────────┘
//	┌─ SERVER ───────────────────────┐  y=36..70
//	│ http://192.168.0.22:8080       │
//	│ Control: connected             │
//	└────────────────────────────────┘
//	┌─ CHANNELS ─────────────────────┐  y=72..130
//	│ ▶ SOURCE opus  [active]        │
//	│ ◀ SINK   opus  [binding]       │
//	└────────────────────────────────┘
//	┌─ SESSION ──────────────────────┐  y=132..166
//	│ abc123  role: translator       │
//	└────────────────────────────────┘
//	┌─ AUDIO ────────────────────────┐  y=168..238
//	│ IN  ████████████░░░░░░░░░░░░░░ │
//	│ OUT ██████░░░░░░░░░░░░░░░░░░░░ │
//	└────────────────────────────────┘
func Render(st *StatusSnapshot) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, screenW, screenH))
	fillRect(img, 0, 0, screenW, screenH, ColorBG)

	y := 1
	y = renderNetwork(img, y, st)
	y += 2
	y = renderServer(img, y, st)
	y += 2
	y = renderChannels(img, y, st)
	y += 2
	y = renderSession(img, y, st)
	y += 2
	renderVU(img, y, st)

	return img
}

func renderNetwork(img *image.RGBA, y int, st *StatusSnapshot) int {
	y = drawPanelHeader(img, y, "NETWORK")

	iface := st.NetworkLink
	if iface == "" {
		iface = "---"
	}
	addr := st.NetworkAddr
	if addr == "" {
		addr = "no address"
	}

	drawLabel(img, pad+2, y, iface)
	drawText(img, pad+2+fontW*7, y, addr)

	statusCol := ColorRed
	statusTxt := "DOWN"
	if st.NetworkUp {
		statusCol = ColorGreen
		statusTxt = "UP"
	}
	drawTextColor(img, screenW-pad-2-StringWidth(statusTxt), y, statusTxt, statusCol)
	y += lineH

	return drawPanelFooter(img, y)
}

func renderServer(img *image.RGBA, y int, st *StatusSnapshot) int {
	y = drawPanelHeader(img, y, "SERVER")

	url := st.ServerURL
	if url == "" {
		url = "(not configured)"
	}
	if len(url) > 48 {
		url = url[:48] + ".."
	}
	drawText(img, pad+2, y, url)
	y += lineH

	drawLabel(img, pad+2, y, "Control:")
	stateCol := ColorRed
	switch st.ControlState {
	case "connected":
		stateCol = ColorGreen
	case "connecting":
		stateCol = ColorYellow
	}
	drawTextColor(img, pad+2+fontW*9, y, st.ControlState, stateCol)
	y += lineH

	return drawPanelFooter(img, y)
}

func renderChannels(img *image.RGBA, y int, st *StatusSnapshot) int {
	y = drawPanelHeader(img, y, "CHANNELS")

	if len(st.Channels) == 0 {
		drawLabel(img, pad+2, y, "(no channels)")
		y += lineH
	}

	maxCh := 4
	for i, ch := range st.Channels {
		if i >= maxCh {
			drawLabel(img, pad+2, y, fmt.Sprintf("  +%d more", len(st.Channels)-maxCh))
			y += lineH
			break
		}

		arrow := ">"
		if ch.Direction == "SINK" {
			arrow = "<"
		}

		codec := ch.Codec
		if codec == "" {
			codec = "opus"
		}

		stateCol := ColorLabel
		switch ch.State {
		case "active":
			stateCol = ColorGreen
		case "binding":
			stateCol = ColorYellow
		case "error":
			stateCol = ColorRed
		}

		drawText(img, pad+2, y, arrow)
		drawLabel(img, pad+2+fontW*2, y, ch.Direction)
		drawText(img, pad+2+fontW*9, y, codec)
		drawTextColor(img, pad+2+fontW*15, y, "["+ch.State+"]", stateCol)
		y += lineH
	}

	return drawPanelFooter(img, y)
}

func renderSession(img *image.RGBA, y int, st *StatusSnapshot) int {
	y = drawPanelHeader(img, y, "SESSION")

	if !st.SessionActive {
		drawLabel(img, pad+2, y, "(no active session)")
		y += lineH
	} else {
		sid := st.SessionID
		if len(sid) > 12 {
			sid = sid[:12] + ".."
		}
		drawText(img, pad+2, y, sid)
		drawLabel(img, pad+2+fontW*15, y, "role:")
		drawText(img, pad+2+fontW*21, y, st.SessionRole)
		y += lineH
	}

	return drawPanelFooter(img, y)
}

func renderVU(img *image.RGBA, y int, st *StatusSnapshot) int {
	y = drawPanelHeader(img, y, "AUDIO")

	barX := pad + 2 + fontW*5
	barW := screenW - barX - pad - 4
	barH := 14

	drawLabel(img, pad+2, y+2, "IN")
	drawVUBar(img, barX, y, barW, barH, st.VUIn)
	y += barH + 4

	drawLabel(img, pad+2, y+2, "OUT")
	drawVUBar(img, barX, y, barW, barH, st.VUOut)
	y += barH + 4

	return drawPanelFooter(img, y)
}

func drawVUBar(img *image.RGBA, x, y, w, h int, level float64) {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}

	fillRect(img, x, y, w, h, ColorVUBG)

	filled := int(float64(w) * level)
	if filled <= 0 {
		return
	}

	greenEnd := w * 60 / 100
	yellowEnd := w * 85 / 100

	for px := 0; px < filled; px++ {
		col := ColorVUGreen
		if px >= yellowEnd {
			col = ColorVURed
		} else if px >= greenEnd {
			col = ColorVUYellow
		}
		for py := 0; py < h; py++ {
			img.SetRGBA(x+px, y+py, col)
		}
	}

	for i := 1; i < 20; i++ {
		segX := x + w*i/20
		if segX < x+filled {
			for py := 0; py < h; py++ {
				img.SetRGBA(segX, y+py, ColorBG)
			}
		}
	}
}

func drawPanelHeader(img *image.RGBA, y int, title string) int {
	fillRect(img, pad, y, screenW-pad*2, lineH, ColorPanel)
	drawTextColor(img, pad+2, y+1, title, ColorBlue)
	drawHLine(img, pad, y+lineH-1, screenW-pad*2, ColorBorder)
	return y + lineH
}

func drawPanelFooter(img *image.RGBA, y int) int {
	drawHLine(img, pad, y, screenW-pad*2, ColorBorder)
	return y + 1
}

func drawLabel(img *image.RGBA, x, y int, s string) {
	DrawString(img, x, y, s, image.Uniform{C: ColorLabel})
}

func drawText(img *image.RGBA, x, y int, s string) {
	DrawString(img, x, y, s, image.Uniform{C: ColorText})
}

func drawTextColor(img *image.RGBA, x, y int, s string, col color.RGBA) {
	DrawString(img, x, y, s, image.Uniform{C: col})
}

func fillRect(img *image.RGBA, x, y, w, h int, col color.RGBA) {
	for py := y; py < y+h && py < img.Bounds().Max.Y; py++ {
		for px := x; px < x+w && px < img.Bounds().Max.X; px++ {
			img.SetRGBA(px, py, col)
		}
	}
}

func drawHLine(img *image.RGBA, x, y, w int, col color.RGBA) {
	for px := x; px < x+w && px < img.Bounds().Max.X; px++ {
		if y >= 0 && y < img.Bounds().Max.Y {
			img.SetRGBA(px, y, col)
		}
	}
}
