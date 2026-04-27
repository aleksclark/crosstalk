package display

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	spiIOCWrMode        = 0x40016B01
	spiIOCWrBitsPerWord = 0x40016B03
	spiIOCWrMaxSpeedHz  = 0x40046B04
	spiIOCMessage1      = 0x40206B00
)

type spiIOCTransfer struct {
	txBuf       uint64
	rxBuf       uint64
	len         uint32
	speedHz     uint32
	delayUsecs  uint16
	bitsPerWord uint8
	csChange    uint8
	txNbits     uint8
	rxNbits     uint8
	wordDelay   uint8
	pad         uint8
}

// Display drives an ILI9341 SPI display directly via /dev/spidev.
type Display struct {
	mu      sync.Mutex
	spiFile *os.File
	dcFile  *os.File
	rstFile *os.File
	buf     []byte
	width   int
	height  int
}

// OpenDisplay opens the SPI device and GPIO pins for the ILI9341.
func OpenDisplay(spiPath string, dcGPIO, rstGPIO int) (*Display, error) {
	spiFile, err := os.OpenFile(spiPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open spi: %w", err)
	}

	fd := spiFile.Fd()
	mode := uint8(0)
	if err := ioctlPtr(fd, spiIOCWrMode, unsafe.Pointer(&mode)); err != nil {
		spiFile.Close()
		return nil, fmt.Errorf("set spi mode: %w", err)
	}
	bits := uint8(8)
	if err := ioctlPtr(fd, spiIOCWrBitsPerWord, unsafe.Pointer(&bits)); err != nil {
		spiFile.Close()
		return nil, fmt.Errorf("set spi bits: %w", err)
	}
	speed := uint32(32000000)
	if err := ioctlPtr(fd, spiIOCWrMaxSpeedHz, unsafe.Pointer(&speed)); err != nil {
		spiFile.Close()
		return nil, fmt.Errorf("set spi speed: %w", err)
	}

	dcFile, err := exportGPIO(dcGPIO, "out")
	if err != nil {
		spiFile.Close()
		return nil, fmt.Errorf("dc gpio %d: %w", dcGPIO, err)
	}

	rstFile, err := exportGPIO(rstGPIO, "out")
	if err != nil {
		spiFile.Close()
		dcFile.Close()
		return nil, fmt.Errorf("reset gpio %d: %w", rstGPIO, err)
	}

	d := &Display{
		spiFile: spiFile,
		dcFile:  dcFile,
		rstFile: rstFile,
		buf:     make([]byte, 320*240*2),
		width:   320,
		height:  240,
	}

	if err := d.init(); err != nil {
		d.Close()
		return nil, fmt.Errorf("display init: %w", err)
	}

	return d, nil
}

func (d *Display) Size() (int, int) {
	return d.width, d.height
}

func (d *Display) Close() error {
	if d.rstFile != nil {
		unexportGPIO(d.rstFile)
	}
	if d.dcFile != nil {
		unexportGPIO(d.dcFile)
	}
	if d.spiFile != nil {
		return d.spiFile.Close()
	}
	return nil
}

func (d *Display) setDC(val int) {
	d.dcFile.Seek(0, 0)
	if val != 0 {
		d.dcFile.Write([]byte("1"))
	} else {
		d.dcFile.Write([]byte("0"))
	}
}

func (d *Display) setReset(val int) {
	d.rstFile.Seek(0, 0)
	if val != 0 {
		d.rstFile.Write([]byte("1"))
	} else {
		d.rstFile.Write([]byte("0"))
	}
}

func (d *Display) spiWrite(data []byte) error {
	for off := 0; off < len(data); {
		chunk := len(data) - off
		if chunk > 4096 {
			chunk = 4096
		}
		xfer := spiIOCTransfer{
			txBuf:   uint64(uintptr(unsafe.Pointer(&data[off]))),
			len:     uint32(chunk),
			speedHz: 32000000,
		}
		if err := ioctlPtr(d.spiFile.Fd(), spiIOCMessage1, unsafe.Pointer(&xfer)); err != nil {
			return fmt.Errorf("spi xfer: %w", err)
		}
		off += chunk
	}
	return nil
}

func (d *Display) writeCmd(cmd byte, data ...byte) {
	d.setDC(0)
	d.spiWrite([]byte{cmd})
	if len(data) > 0 {
		d.setDC(1)
		d.spiWrite(data)
	}
}

func (d *Display) init() error {
	slog.Info("ili9341: resetting display")
	d.setReset(1)
	time.Sleep(50 * time.Millisecond)
	d.setReset(0)
	time.Sleep(50 * time.Millisecond)
	d.setReset(1)
	time.Sleep(150 * time.Millisecond)

	slog.Info("ili9341: sending init sequence")
	d.writeCmd(0x01)
	time.Sleep(150 * time.Millisecond)

	d.writeCmd(0xCB, 0x39, 0x2C, 0x00, 0x34, 0x02)
	d.writeCmd(0xCF, 0x00, 0xC1, 0x30)
	d.writeCmd(0xE8, 0x85, 0x00, 0x78)
	d.writeCmd(0xEA, 0x00, 0x00)
	d.writeCmd(0xED, 0x64, 0x03, 0x12, 0x81)
	d.writeCmd(0xF7, 0x20)
	d.writeCmd(0xC0, 0x23)
	d.writeCmd(0xC1, 0x10)
	d.writeCmd(0xC5, 0x3E, 0x28)
	d.writeCmd(0xC7, 0x86)
	d.writeCmd(0x36, 0x28) // MADCTL: landscape + BGR
	d.writeCmd(0x3A, 0x55) // 16-bit color
	d.writeCmd(0xB1, 0x00, 0x18)
	d.writeCmd(0xB6, 0x08, 0x82, 0x27)
	d.writeCmd(0xF2, 0x00)
	d.writeCmd(0x26, 0x01)
	d.writeCmd(0x11) // Sleep out
	time.Sleep(150 * time.Millisecond)
	d.writeCmd(0x29) // Display ON
	time.Sleep(50 * time.Millisecond)

	slog.Info("ili9341: display initialized")
	return nil
}

// Flush converts an RGBA image to RGB565 and sends it over SPI.
func (d *Display) Flush(img *image.RGBA) {
	d.mu.Lock()
	defer d.mu.Unlock()

	bounds := img.Bounds()
	maxX := bounds.Max.X
	maxY := bounds.Max.Y
	if maxX > d.width {
		maxX = d.width
	}
	if maxY > d.height {
		maxY = d.height
	}

	for y := 0; y < maxY; y++ {
		imgRow := y * img.Stride
		bufRow := y * d.width * 2
		for x := 0; x < maxX; x++ {
			off := imgRow + x*4
			r := uint16(img.Pix[off])
			g := uint16(img.Pix[off+1])
			b := uint16(img.Pix[off+2])
			pixel := (r>>3)<<11 | (g>>2)<<5 | (b >> 3)
			bOff := bufRow + x*2
			binary.BigEndian.PutUint16(d.buf[bOff:], pixel)
		}
	}

	// Set window
	d.writeCmd(0x2A, 0x00, 0x00, byte((d.width-1)>>8), byte(d.width-1))
	d.writeCmd(0x2B, 0x00, 0x00, byte((d.height-1)>>8), byte(d.height-1))

	// Memory write
	d.setDC(0)
	d.spiWrite([]byte{0x2C})
	d.setDC(1)
	d.spiWrite(d.buf[:d.width*d.height*2])
}

func exportGPIO(num int, dir string) (*os.File, error) {
	numStr := fmt.Sprintf("%d", num)
	valPath := fmt.Sprintf("/sys/class/gpio/gpio%d/value", num)

	// If GPIO is already exported (e.g. by systemd ExecStartPre), just open it
	if _, err := os.Stat(valPath); err == nil {
		return os.OpenFile(valPath, os.O_WRONLY, 0)
	}

	// Try to export
	if f, err := os.OpenFile("/sys/class/gpio/export", os.O_WRONLY, 0); err == nil {
		f.Write([]byte(numStr))
		f.Close()
		time.Sleep(50 * time.Millisecond)
	} else {
		return nil, fmt.Errorf("export gpio: %w", err)
	}

	dirPath := fmt.Sprintf("/sys/class/gpio/gpio%d/direction", num)
	if err := os.WriteFile(dirPath, []byte(dir), 0644); err != nil {
		return nil, fmt.Errorf("set direction: %w", err)
	}

	return os.OpenFile(valPath, os.O_WRONLY, 0)
}

func unexportGPIO(valFile *os.File) {
	valFile.Close()
}

func ioctlPtr(fd uintptr, req uint, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

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
