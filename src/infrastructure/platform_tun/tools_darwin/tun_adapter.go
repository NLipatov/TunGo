package tools_darwin

import (
	"encoding/binary"
	"errors"
	"golang.zx2c4.com/wireguard/tun"
	"syscall"
	"tungo/application"
)

type DarwinWgTunAdapter struct{ tun.Device }

func NewWgTunAdapter(dev tun.Device) application.TunDevice { return &DarwinWgTunAdapter{Device: dev} }

// Read returns a clean IP packet, utun header already stripped.
func (a *DarwinWgTunAdapter) Read(p []byte) (int, error) {
	tmp := make([]byte, len(p)+4) // +4 под utun-заголовок
	bufs, sizes := [][]byte{tmp}, []int{0}

	if _, err := a.Device.Read(bufs, sizes, 4); err != nil {
		return 0, err
	}
	n := sizes[0]       // размер IP-пакета
	copy(p, tmp[4:4+n]) // переносим без сдвига внутри самого p

	return n, nil
}

// Write prepends utun header and pushes the packet into the kernel.
func (a *DarwinWgTunAdapter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("empty packet")
	}

	buf := make([]byte, len(p)+4)
	var family uint32
	if p[0]>>4 == 6 {
		family = syscall.AF_INET6
	} else {
		family = syscall.AF_INET
	}
	binary.BigEndian.PutUint32(buf[:4], family) // utun header
	copy(buf[4:], p)

	if _, err := a.Device.Write([][]byte{buf}, 4); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *DarwinWgTunAdapter) Close() error { return a.Device.Close() }
