package can_interface

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	structs "github.com/eg4/battery/monitor/internal/data-structures"
)

// Pure-stdlib AF_CAN / CAN_RAW socket bound to an interface (e.g. "can0").
const (
	afCAN        = 29     // AF_CAN / PF_CAN
	canRAW       = 1      // CAN_RAW protocol
	siocGIFINDEX = 0x8933 // ioctl: name -> ifindex
	canEFFFlag   = 0x80000000
	canRTRFlag   = 0x40000000
	canERRFlag   = 0x20000000
	canEFFMask   = 0x1FFFFFFF
	canSFFMask   = 0x000007FF
	canFrameLen  = 16 // struct can_frame: id(4) dlc(1) pad(3) data(8)
)

// struct sockaddr_can { u16 family; int ifindex; union{...} addr; } = 16 bytes
type sockaddrCAN struct {
	family  uint16
	_       [2]byte
	ifindex int32
	addr    [8]byte
}

// struct ifreq (only name + ifr_ifindex used here), padded to 40 bytes.
type ifreq struct {
	name  [16]byte
	index int32
	_     [20]byte
}

// TestPort checks the connectivity status of the specified network interface and sends the result to the provided channel.
func TestPort(iface string, connectedChannel chan bool) {

	fmt.Fprintf(os.Stderr, "Trying to connect to CAN Interface: %s (SocketCAN)\n", iface)

	fd, err := openCAN(iface)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		connectedChannel <- false
		return
	}

	f := os.NewFile(uintptr(fd), iface) // integrates with Go's runtime poller
	defer f.Close()
	fmt.Fprintf(os.Stderr, "connected: %s (SocketCAN)\n", iface)
	connectedChannel <- true

}

func HardwareSource(out chan<- structs.Frame, iface string) {

	fd, err := openCAN(iface)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		close(out)
		return
	}
	f := os.NewFile(uintptr(fd), iface) // integrates with Go's runtime poller
	defer f.Close()
	fmt.Fprintf(os.Stderr, "connected: %s (SocketCAN)\n", iface)

	buf := make([]byte, 72) // large enough for CAN FD too; classic frames are 16
	for {
		n, err := f.Read(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			close(out)
			return
		}
		if n < canFrameLen {
			continue
		}
		// can_id and dlc are host byte order; assume little-endian (x86/ARM).
		rawID := binary.LittleEndian.Uint32(buf[0:4])
		if rawID&canERRFlag != 0 {
			continue // error frame, not real bus data
		}
		dlc := min(int(buf[4]&0x0F), 8)
		var id uint32
		if rawID&canEFFFlag != 0 {
			id = rawID & canEFFMask
		} else {
			id = rawID & canSFFMask
		}
		data := make([]byte, dlc)
		copy(data, buf[8:8+dlc])
		out <- structs.Frame{Id: id, Data: data}
	}
}

func openCAN(iface string) (int, error) {
	fd, err := syscall.Socket(afCAN, syscall.SOCK_RAW, canRAW)
	if err != nil {
		return -1, fmt.Errorf("socket(AF_CAN): %w", err)
	}
	idx, err := ifIndex(fd, iface)
	if err != nil {
		syscall.Close(fd)
		return -1, fmt.Errorf("interface %q: %w", iface, err)
	}
	sa := sockaddrCAN{family: afCAN, ifindex: idx}
	_, _, e := syscall.Syscall(syscall.SYS_BIND, uintptr(fd),
		uintptr(unsafe.Pointer(&sa)), unsafe.Sizeof(sa))
	if e != 0 {
		syscall.Close(fd)
		return -1, fmt.Errorf("bind %q: %w", iface, e)
	}
	return fd, nil
}

func ifIndex(fd int, name string) (int32, error) {
	var req ifreq
	if len(name) >= len(req.name) {
		return 0, fmt.Errorf("interface name too long: %q", name)
	}
	copy(req.name[:], name)
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd),
		uintptr(siocGIFINDEX), uintptr(unsafe.Pointer(&req)))
	if e != 0 {
		return 0, e
	}
	return req.index, nil
}
