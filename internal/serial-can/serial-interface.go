package serial_can

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"
)

// ---- Waveshare USB-CAN-A serial link (Linux termios2 for 2 Mbaud) ----------
const (
	tcgets2 = 0x802C542A // _IOR('T', 0x2A, struct termios2)
	tcsets2 = 0x402C542B // _IOW('T', 0x2B, struct termios2)
	bother  = 0x1000
	cbaud   = 0x100F
)

// Frame ---------------------------------------------------------------------------
// Frame CAN Data Frame
// ---------------------------------------------------------------------------
type Frame struct {
	Id   uint32
	Data []byte
}

type termios2 struct {
	Iflag, Oflag, Cflag, Lflag uint32
	Line                       uint8
	Cc                         [19]uint8
	Ispeed, Ospeed             uint32
}

func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

// configureSerial puts the port in raw mode at an arbitrary baud via termios2.
func configureSerial(fd int, baud uint32) error {
	var t termios2
	if err := ioctl(fd, tcgets2, unsafe.Pointer(&t)); err != nil {
		return fmt.Errorf("TCGETS2: %w", err)
	}
	t.Iflag = 0
	t.Oflag = 0
	t.Lflag = 0
	t.Cflag &^= cbaud | syscall.CSIZE | syscall.PARENB | syscall.CSTOPB
	t.Cflag |= syscall.CS8 | syscall.CREAD | syscall.CLOCAL | bother
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	t.Ispeed = baud
	t.Ospeed = baud
	if err := ioctl(fd, tcsets2, unsafe.Pointer(&t)); err != nil {
		return fmt.Errorf("TCSETS2: %w", err)
	}
	return nil
}

// bitrate byte codes accepted by the adapter's config frame.
var bitrateCode = map[int]byte{
	1000000: 0x01, 800000: 0x02, 500000: 0x03, 400000: 0x04,
	250000: 0x05, 200000: 0x06, 125000: 0x07, 100000: 0x08,
	50000: 0x09, 20000: 0x0A, 10000: 0x0B, 5000: 0x0C,
}
var modeCode = map[string]byte{"normal": 0x00, "silent": 0x02}

// configFrame is the 20-byte 0xAA 0x55 0x12 ... init packet (matches python-can).
func configFrame(bitrate int, mode string) ([]byte, error) {
	bc, ok := bitrateCode[bitrate]
	if !ok {
		return nil, fmt.Errorf("unsupported CAN bitrate %d", bitrate)
	}
	mc := modeCode[mode]
	m := []byte{
		0xAA, 0x55, 0x12,
		bc,         // CAN baud
		0x01,       // frame type: STD
		0, 0, 0, 0, // filter id
		0, 0, 0, 0, // mask id
		mc,   // operation mode
		0x01, // "send once" flag, per vendor app
		0, 0, 0, 0,
	}
	var sum byte
	for _, b := range m[2:] {
		sum += b
	}
	return append(m, sum), nil
}

func HardwareSource(out chan<- Frame, channel string, bitrate, serialBaud int, mode string) {
	fd, err := syscall.Open(channel, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s failed: %v\n", channel, err)
		close(out)
		return
	}
	if err := configureSerial(fd, uint32(serialBaud)); err != nil {
		fmt.Fprintf(os.Stderr, "configure failed: %v\n", err)
		syscall.Close(fd)
		close(out)
		return
	}
	f := os.NewFile(uintptr(fd), channel)
	defer f.Close()

	// Send the adapter init/config frame (sets CAN bitrate + mode).
	cfg, err := configFrame(bitrate, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		close(out)
		return
	}
	if _, err := f.Write(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "write init frame failed: %v\n", err)
		close(out)
		return
	}
	fmt.Fprintf(os.Stderr, "connected: %s @ %d bps CAN (%s)\n", channel, bitrate, mode)

	r := bufio.NewReaderSize(f, 4096)
	for {
		// resync to start byte 0xAA
		b1, err := r.ReadByte()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			close(out)
			return
		}
		if b1 != 0xAA {
			continue
		}
		b2, err := r.ReadByte()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			close(out)
			return
		}
		if b2 == 0x55 { // status frame: 0xAA 0x55 + 18 bytes, ignore
			io.CopyN(io.Discard, r, 18)
			continue
		}
		length := int(b2 & 0x0F)
		isExt := b2&0x20 != 0
		var id uint32
		if isExt {
			var idb [4]byte
			if _, err := io.ReadFull(r, idb[:]); err != nil {
				continue
			}
			id = binary.LittleEndian.Uint32(idb[:])
		} else {
			var idb [2]byte
			if _, err := io.ReadFull(r, idb[:]); err != nil {
				continue
			}
			id = uint32(binary.LittleEndian.Uint16(idb[:]))
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			continue
		}
		end, err := r.ReadByte()
		if err != nil || end != 0x55 {
			continue // framing error -> resync
		}
		out <- Frame{id, data}
	}
}
