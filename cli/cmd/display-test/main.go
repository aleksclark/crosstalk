package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type fbVarScreeninfo struct {
	XRes, YRes                   uint32
	XResVirtual, YResVirtual     uint32
	XOffset, YOffset             uint32
	BitsPerPixel, Grayscale      uint32
	Red, Green, Blue, Transp     fbBitfield
}

type fbBitfield struct {
	Offset, Length, Right uint32
}

const (
	fbioGetVScreenInfo = 0x4600
)

func main() {
	fbPath := "/dev/fb1"
	if len(os.Args) > 1 {
		fbPath = os.Args[1]
	}

	fmt.Println("Opening framebuffer:", fbPath)
	f, err := os.OpenFile(fbPath, os.O_RDWR, 0)
	if err != nil {
		fmt.Println("ERROR:", err)
		fmt.Println("Trying /dev/fb0...")
		fbPath = "/dev/fb0"
		f, err = os.OpenFile(fbPath, os.O_RDWR, 0)
		if err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	}
	defer f.Close()

	var vinfo fbVarScreeninfo
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL,
		f.Fd(), fbioGetVScreenInfo,
		uintptr(unsafe.Pointer(&vinfo)),
	); errno != 0 {
		fmt.Println("ioctl VSCREENINFO error:", errno)
		os.Exit(1)
	}

	w := int(vinfo.XRes)
	h := int(vinfo.YRes)
	bpp := int(vinfo.BitsPerPixel) / 8
	fmt.Printf("Resolution: %dx%d, %dbpp\n", w, h, vinfo.BitsPerPixel)

	size := w * h * bpp
	data, err := unix.Mmap(int(f.Fd()), 0, size,
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		fmt.Println("mmap error:", err)
		os.Exit(1)
	}
	defer unix.Munmap(data)

	fill := func(name string, r, g, b uint16) {
		fmt.Printf("Filling %s...\n", name)
		pixel := (r>>3)<<11 | (g>>2)<<5 | (b >> 3)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				off := (y*w + x) * bpp
				if bpp == 2 {
					binary.LittleEndian.PutUint16(data[off:], pixel)
				}
			}
		}
	}

	fill("RED", 0xFF, 0x00, 0x00)
	time.Sleep(2 * time.Second)

	fill("GREEN", 0x00, 0xFF, 0x00)
	time.Sleep(2 * time.Second)

	fill("BLUE", 0x00, 0x00, 0xFF)
	time.Sleep(2 * time.Second)

	fmt.Println("Done. Display should have shown R/G/B.")
}
