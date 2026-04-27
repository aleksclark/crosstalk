package display

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fbVarScreeninfo matches the Linux struct fb_var_screeninfo (first 14 fields).
type fbVarScreeninfo struct {
	XRes         uint32
	YRes         uint32
	XResVirtual  uint32
	YResVirtual  uint32
	XOffset      uint32
	YOffset      uint32
	BitsPerPixel uint32
	Grayscale    uint32
	Red          fbBitfield
	Green        fbBitfield
	Blue         fbBitfield
	Transp       fbBitfield
}

// fbFixScreeninfo matches the first fields of Linux struct fb_fix_screeninfo.
type fbFixScreeninfo struct {
	ID         [16]byte
	SMEMStart  uint64
	SMEMLen    uint32
	Type       uint32
	TypeAux    uint32
	Visual     uint32
	XPanStep   uint16
	YPanStep   uint16
	YWrapStep  uint16
	_          uint16
	LineLength uint32
}

type fbBitfield struct {
	Offset uint32
	Length uint32
	Right  uint32
}

const (
	fbioGetVScreenInfo = 0x4600
	fbioPutVScreenInfo = 0x4601
	fbioGetFScreenInfo = 0x4602
	fbioPanDisplay    = 0x4606
	fbioBlank         = 0x4611
)

// Display drives an ILI9341 via the Linux framebuffer (/dev/fb).
// The kernel's tinydrm ili9341 driver handles SPI communication;
// we just mmap the framebuffer and write XRGB8888 pixels, then
// issue FBIOPAN_DISPLAY to trigger the DRM dirty-rect flush over SPI.
type Display struct {
	mu     sync.Mutex
	file   *os.File
	fbData []byte
	vinfo  fbVarScreeninfo
	width  int
	height int
	stride int
	bpp    int
}

// OpenDisplay opens the framebuffer device. It probes /dev/fb1 first
// (typical for SPI displays when HDMI is fb0), then falls back to /dev/fb0.
// Pass an explicit path to override.
func OpenDisplay(fbPath string) (*Display, error) {
	if fbPath == "" {
		for _, candidate := range []string{"/dev/fb1", "/dev/fb0"} {
			if _, err := os.Stat(candidate); err == nil {
				fbPath = candidate
				break
			}
		}
		if fbPath == "" {
			return nil, fmt.Errorf("no framebuffer device found")
		}
	}

	file, err := os.OpenFile(fbPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", fbPath, err)
	}

	var vinfo fbVarScreeninfo
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL,
		file.Fd(), fbioGetVScreenInfo,
		uintptr(unsafe.Pointer(&vinfo)),
	); errno != 0 {
		file.Close()
		return nil, fmt.Errorf("FBIOGET_VSCREENINFO %s: %w", fbPath, errno)
	}

	var finfo fbFixScreeninfo
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL,
		file.Fd(), fbioGetFScreenInfo,
		uintptr(unsafe.Pointer(&finfo)),
	); errno != 0 {
		file.Close()
		return nil, fmt.Errorf("FBIOGET_FSCREENINFO %s: %w", fbPath, errno)
	}

	bpp := int(vinfo.BitsPerPixel) / 8
	stride := int(finfo.LineLength)
	size := stride * int(vinfo.YRes)

	data, err := unix.Mmap(int(file.Fd()), 0, size,
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("mmap %s: %w", fbPath, err)
	}

	d := &Display{
		file:   file,
		fbData: data,
		vinfo:  vinfo,
		width:  int(vinfo.XRes),
		height: int(vinfo.YRes),
		stride: stride,
		bpp:    bpp,
	}

	// Unblank and trigger initial modeset so tinydrm starts flushing.
	unix.Syscall(unix.SYS_IOCTL, file.Fd(), fbioBlank, 0)

	return d, nil
}

// Size returns the display resolution.
func (d *Display) Size() (int, int) {
	return d.width, d.height
}

// Close unmaps and closes the framebuffer.
func (d *Display) Close() error {
	if d.fbData != nil {
		for i := range d.fbData {
			d.fbData[i] = 0
		}
		unix.Munmap(d.fbData)
		d.fbData = nil
	}
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}

// Flush converts an RGBA image to the framebuffer's pixel format and
// writes it. Supports RGB565 (16bpp, tinydrm default) and BGRA8888 (32bpp).
func (d *Display) Flush(img *image.RGBA) {
	d.mu.Lock()
	defer d.mu.Unlock()

	bounds := img.Bounds()
	maxX := min(bounds.Max.X, d.width)
	maxY := min(bounds.Max.Y, d.height)

	switch d.bpp {
	case 2: // RGB565
		for y := 0; y < maxY; y++ {
			imgRow := y * img.Stride
			fbRow := y * d.stride
			for x := 0; x < maxX; x++ {
				off := imgRow + x*4
				r := uint16(img.Pix[off])
				g := uint16(img.Pix[off+1])
				b := uint16(img.Pix[off+2])
				pixel := (r>>3)<<11 | (g>>2)<<5 | (b >> 3)
				bOff := fbRow + x*2
				if bOff+1 < len(d.fbData) {
					binary.LittleEndian.PutUint16(d.fbData[bOff:], pixel)
				}
			}
		}
	case 4: // XRGB8888 (DRM fbdev emulation)
		for y := 0; y < maxY; y++ {
			imgRow := y * img.Stride
			fbRow := y * d.stride
			for x := 0; x < maxX; x++ {
				off := imgRow + x*4
				bOff := fbRow + x*4
				if bOff+3 < len(d.fbData) {
					d.fbData[bOff] = img.Pix[off+2]   // B
					d.fbData[bOff+1] = img.Pix[off+1] // G
					d.fbData[bOff+2] = img.Pix[off]   // R
					d.fbData[bOff+3] = 0xFF            // X
				}
			}
		}
	}

	// Trigger tinydrm dirty-rect flush over SPI.
	unix.Syscall(unix.SYS_IOCTL, d.file.Fd(),
		fbioPanDisplay, uintptr(unsafe.Pointer(&d.vinfo)))
}

// Exported color palette — unchanged from the SPI version.
var (
	ColorBG       = color.RGBA{R: 0x0a, G: 0x0a, B: 0x14, A: 0xff}
	ColorPanel    = color.RGBA{R: 0x14, G: 0x1e, B: 0x28, A: 0xff}
	ColorBorder   = color.RGBA{R: 0x2a, G: 0x3a, B: 0x4a, A: 0xff}
	ColorText     = color.RGBA{R: 0xd0, G: 0xd8, B: 0xe0, A: 0xff}
	ColorLabel    = color.RGBA{R: 0x70, G: 0x80, B: 0x90, A: 0xff}
	ColorGreen    = color.RGBA{R: 0x00, G: 0xc8, B: 0x50, A: 0xff}
	ColorRed      = color.RGBA{R: 0xe0, G: 0x30, B: 0x30, A: 0xff}
	ColorYellow   = color.RGBA{R: 0xe0, G: 0xc0, B: 0x00, A: 0xff}
	ColorBlue     = color.RGBA{R: 0x40, G: 0x80, B: 0xe0, A: 0xff}
	ColorVUGreen  = color.RGBA{R: 0x00, G: 0xd0, B: 0x40, A: 0xff}
	ColorVUYellow = color.RGBA{R: 0xe0, G: 0xd0, B: 0x00, A: 0xff}
	ColorVURed    = color.RGBA{R: 0xe0, G: 0x20, B: 0x20, A: 0xff}
	ColorVUBG     = color.RGBA{R: 0x18, G: 0x20, B: 0x28, A: 0xff}
)
