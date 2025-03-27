package tun_device

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"sync"
)

// wintunTun is windows-specific TUN implementation via wintun driver(see https://www.wintun.net)
type wintunTun struct {
	adapter wintun.Adapter
	session *wintun.Session
	name    string
	mtu     int
	closeCh chan struct{}
	once    sync.Once
}

func (d *wintunTun) Read(data []byte) (int, error) {
	event := d.session.ReadWaitEvent()

	for {
		select {
		case <-d.closeCh:
			return 0, errors.New("tun device closed")
		default:
			packet, err := d.session.ReceivePacket()
			if err == nil {
				n := copy(data, packet)
				d.session.ReleaseReceivePacket(packet)
				return n, nil
			}
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				// Here, timeout is used to periodically unblock the WaitForSingleObject call,
				// allowing the loop to check if the TUN interface has been closed via closeCh.
				var timeout uint32 = 500
				_, _ = windows.WaitForSingleObject(event, timeout)
				continue
			}
			return 0, err
		}
	}
}

func (d *wintunTun) Close() error {
	d.once.Do(func() {
		close(d.closeCh)
		_ = windows.SetEvent(d.session.ReadWaitEvent())
		d.session.End()
		_ = d.adapter.Close()
	})
	return nil
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
