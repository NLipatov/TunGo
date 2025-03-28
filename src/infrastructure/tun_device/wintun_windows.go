package tun_device

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"sync"
)

// wintunTun is windows-specific TUN implementation via wintun driver(see https://www.wintun.net)
type wintunTun struct {
	adapter   wintun.Adapter
	session   *wintun.Session
	name      string
	mtu       int
	closeCh   chan struct{}
	once      sync.Once
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
}

func (d *wintunTun) Read(data []byte) (int, error) {
	event := d.session.ReadWaitEvent()

	for {
		select {
		case <-d.closeCh:
			return 0, errors.New("tun device closed")
		default:
			// Wait before attempting ReceivePacket
			var timeout uint32 = 500
			status, _ := windows.WaitForSingleObject(event, timeout)
			if status == windows.WAIT_OBJECT_0 {
				// Only then try ReceivePacket
				select {
				case <-d.closeCh:
					return 0, errors.New("tun device closed")
				default:
					packet, err := d.session.ReceivePacket()
					if err != nil {
						if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
							continue
						}
						return 0, err
					}
					n := copy(data, packet)
					d.session.ReleaseReceivePacket(packet)
					return n, nil
				}
			}
		}
	}
}

func (d *wintunTun) Write(data []byte) (int, error) {
	packet, err := d.session.AllocateSendPacket(len(data))
	if err != nil {
		return 0, err
	}
	copy(packet, data)
	d.session.SendPacket(packet)
	return len(data), nil
}

func (w *wintunTun) Close() error {
	w.closeMu.Lock()
	defer w.closeMu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true
	close(w.closeCh)

	w.closeOnce.Do(func() {
		w.session.End()
	})
	return nil
}
