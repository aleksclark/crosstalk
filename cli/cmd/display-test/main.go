package main

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	spiIOCWrMode       = 0x40016B01
	spiIOCWrBits       = 0x40016B03
	spiIOCWrSpeed      = 0x40046B04
	spiIOCMessage1     = 0x40206B00
)

type spiTransfer struct {
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

var (
	spiFile *os.File
	dcFile  *os.File
	rstFile *os.File
)

func main() {
	spiPath := "/dev/spidev0.1"
	if len(os.Args) > 1 {
		spiPath = os.Args[1]
	}

	fmt.Println("Opening SPI:", spiPath)
	var err error
	spiFile, err = os.OpenFile(spiPath, os.O_RDWR, 0)
	if err != nil {
		fmt.Println("ERROR open spi:", err)
		os.Exit(1)
	}
	defer spiFile.Close()

	fd := spiFile.Fd()
	mode := uint8(0)
	ioctl(fd, spiIOCWrMode, unsafe.Pointer(&mode))
	bits := uint8(8)
	ioctl(fd, spiIOCWrBits, unsafe.Pointer(&bits))
	speed := uint32(32000000)
	ioctl(fd, spiIOCWrSpeed, unsafe.Pointer(&speed))
	fmt.Println("SPI configured: mode=0, 8bit, 32MHz")

	dcFile, err = openGPIO(71)
	if err != nil {
		fmt.Println("ERROR dc gpio:", err)
		os.Exit(1)
	}
	defer dcFile.Close()
	fmt.Println("DC GPIO 71 opened")

	rstFile, err = openGPIO(76)
	if err != nil {
		fmt.Println("ERROR rst gpio:", err)
		os.Exit(1)
	}
	defer rstFile.Close()
	fmt.Println("RST GPIO 76 opened")

	fmt.Println("Resetting display...")
	gpioSet(rstFile, 1)
	time.Sleep(50 * time.Millisecond)
	gpioSet(rstFile, 0)
	time.Sleep(50 * time.Millisecond)
	gpioSet(rstFile, 1)
	time.Sleep(150 * time.Millisecond)

	fmt.Println("Sending init sequence...")
	cmd(0x01)
	time.Sleep(150 * time.Millisecond)

	cmd(0xCB, 0x39, 0x2C, 0x00, 0x34, 0x02)
	cmd(0xCF, 0x00, 0xC1, 0x30)
	cmd(0xE8, 0x85, 0x00, 0x78)
	cmd(0xEA, 0x00, 0x00)
	cmd(0xED, 0x64, 0x03, 0x12, 0x81)
	cmd(0xF7, 0x20)
	cmd(0xC0, 0x23)
	cmd(0xC1, 0x10)
	cmd(0xC5, 0x3E, 0x28)
	cmd(0xC7, 0x86)
	cmd(0x36, 0x28)
	cmd(0x3A, 0x55)
	cmd(0xB1, 0x00, 0x18)
	cmd(0xB6, 0x08, 0x82, 0x27)
	cmd(0xF2, 0x00)
	cmd(0x26, 0x01)
	cmd(0x11)
	time.Sleep(150 * time.Millisecond)
	cmd(0x29)
	time.Sleep(50 * time.Millisecond)
	fmt.Println("Init complete")

	fmt.Println("Filling RED...")
	fillColor(0xF8, 0x00)
	time.Sleep(2 * time.Second)

	fmt.Println("Filling GREEN...")
	fillColor(0x07, 0xE0)
	time.Sleep(2 * time.Second)

	fmt.Println("Filling BLUE...")
	fillColor(0x00, 0x1F)
	time.Sleep(2 * time.Second)

	fmt.Println("Done. Display should have shown R/G/B.")
}

func fillColor(hi, lo byte) {
	cmd(0x2A, 0x00, 0x00, 0x01, 0x3F)
	cmd(0x2B, 0x00, 0x00, 0x00, 0xEF)
	gpioSet(dcFile, 0)
	spiWrite([]byte{0x2C})
	gpioSet(dcFile, 1)
	buf := make([]byte, 320*240*2)
	for i := 0; i < len(buf); i += 2 {
		buf[i] = hi
		buf[i+1] = lo
	}
	spiWrite(buf)
}

func cmd(c byte, data ...byte) {
	gpioSet(dcFile, 0)
	spiWrite([]byte{c})
	if len(data) > 0 {
		gpioSet(dcFile, 1)
		spiWrite(data)
	}
}

func spiWrite(data []byte) {
	for off := 0; off < len(data); {
		chunk := len(data) - off
		if chunk > 4096 {
			chunk = 4096
		}
		xfer := spiTransfer{
			txBuf:   uint64(uintptr(unsafe.Pointer(&data[off]))),
			len:     uint32(chunk),
			speedHz: 32000000,
		}
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, spiFile.Fd(), uintptr(spiIOCMessage1), uintptr(unsafe.Pointer(&xfer)))
		if errno != 0 {
			fmt.Println("SPI write error:", errno)
			return
		}
		off += chunk
	}
}

func gpioSet(f *os.File, val int) {
	f.Seek(0, 0)
	if val != 0 {
		f.Write([]byte("1"))
	} else {
		f.Write([]byte("0"))
	}
}

func openGPIO(num int) (*os.File, error) {
	valPath := fmt.Sprintf("/sys/class/gpio/gpio%d/value", num)
	if _, err := os.Stat(valPath); err == nil {
		return os.OpenFile(valPath, os.O_WRONLY, 0)
	}
	return nil, fmt.Errorf("gpio%d not exported (run as root: echo %d > /sys/class/gpio/export)", num, num)
}

func ioctl(fd uintptr, req uint, arg unsafe.Pointer) {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), uintptr(arg))
	if errno != 0 {
		fmt.Println("ioctl error:", errno)
	}
}
