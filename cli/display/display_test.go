package display

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_EmptyStatus(t *testing.T) {
	st := &StatusSnapshot{ControlState: "disconnected"}
	img := Render(st)

	require.NotNil(t, img)
	assert.Equal(t, 320, img.Bounds().Dx())
	assert.Equal(t, 240, img.Bounds().Dy())
}

func TestRender_FullStatus(t *testing.T) {
	st := &StatusSnapshot{
		NetworkLink:  "wlan0",
		NetworkAddr:  "192.168.0.109",
		NetworkUp:    true,
		ServerURL:    "http://192.168.0.22:8080",
		ControlState: "connected",
		Channels: []ChannelInfo{
			{ID: "ch1", Direction: "SOURCE", Codec: "opus", State: "active"},
			{ID: "ch2", Direction: "SINK", Codec: "opus", State: "binding"},
		},
		SessionID:     "abc123",
		SessionRole:   "translator",
		SessionActive: true,
		VUIn:          0.7,
		VUOut:         0.3,
	}

	img := Render(st)
	require.NotNil(t, img)
	assert.Equal(t, 320, img.Bounds().Dx())
	assert.Equal(t, 240, img.Bounds().Dy())

	assert.False(t, isUniform(img), "rendered image should not be a single solid color")
}

func TestRender_VUBars(t *testing.T) {
	stZero := &StatusSnapshot{ControlState: "disconnected", VUIn: 0, VUOut: 0}
	stFull := &StatusSnapshot{ControlState: "disconnected", VUIn: 1.0, VUOut: 1.0}

	imgZero := Render(stZero)
	imgFull := Render(stFull)

	greenPixelsZero := countColor(imgZero, ColorVUGreen)
	greenPixelsFull := countColor(imgFull, ColorVUGreen)

	assert.Greater(t, greenPixelsFull, greenPixelsZero, "full VU should have more green pixels")
}

func TestStatusSnapshot(t *testing.T) {
	s := &Status{}
	s.SetNetwork("lo", "1.2.3.4", true)
	s.UpsertChannel(ChannelInfo{ID: "ch1", Direction: "SOURCE"})

	snap := s.Snapshot()
	s.SetNetwork("eth0", "5.6.7.8", true)
	s.UpsertChannel(ChannelInfo{ID: "ch2", Direction: "SINK"})

	assert.Equal(t, "1.2.3.4", snap.NetworkAddr)
	assert.Len(t, snap.Channels, 1)
}

func TestStatusUpsertChannel(t *testing.T) {
	s := &Status{}

	s.UpsertChannel(ChannelInfo{ID: "ch1", State: "binding"})
	snap := s.Snapshot()
	require.Len(t, snap.Channels, 1)
	assert.Equal(t, "binding", snap.Channels[0].State)

	s.UpsertChannel(ChannelInfo{ID: "ch1", State: "active"})
	snap = s.Snapshot()
	require.Len(t, snap.Channels, 1)
	assert.Equal(t, "active", snap.Channels[0].State)
}

func TestStatusRemoveChannel(t *testing.T) {
	s := &Status{}
	s.UpsertChannel(ChannelInfo{ID: "ch1"})
	s.UpsertChannel(ChannelInfo{ID: "ch2"})

	s.RemoveChannel("ch1")
	snap := s.Snapshot()
	require.Len(t, snap.Channels, 1)
	assert.Equal(t, "ch2", snap.Channels[0].ID)
}

func TestDrawString(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 60, 12))
	DrawString(img, 0, 0, "Hi", image.Uniform{C: color.RGBA{R: 0xff, A: 0xff}})

	hasColor := false
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if r > 0 {
				hasColor = true
			}
		}
	}
	assert.True(t, hasColor, "DrawString should produce visible pixels")
}

func TestProbeNetwork(t *testing.T) {
	iface, addr, up := ProbeNetwork()
	if up {
		assert.NotEmpty(t, iface)
		assert.NotEmpty(t, addr)
	}
}

func isUniform(img *image.RGBA) bool {
	first := img.RGBAAt(0, 0)
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y) != first {
				return false
			}
		}
	}
	return true
}

func countColor(img *image.RGBA, target color.RGBA) int {
	count := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y) == target {
				count++
			}
		}
	}
	return count
}
