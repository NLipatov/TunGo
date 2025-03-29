package tun_device

import (
	"errors"
	"fmt"
	"log"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"tungo/application"
)

// wintunTun представляет Windows-TUN устройство, использующее драйвер wintun.
type wintunTun struct {
	adapter    wintun.Adapter
	session    *wintun.Session
	name       string
	closeEvent windows.Handle
	closed     bool
}

func newWinTun(adapter wintun.Adapter, session wintun.Session) application.TunDevice {
	handle, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		log.Println("Error creating winTun handle:", err)
	}
	return &wintunTun{
		adapter:    adapter,
		session:    &session,
		closeEvent: handle,
	}
}

func (d *wintunTun) Read(data []byte) (int, error) {
	event := d.session.ReadWaitEvent()

	for {
		if d.closed {
			return 0, fmt.Errorf("session closed")
		}

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

func (t *wintunTun) Write(data []byte) (int, error) {
	packet, err := t.session.AllocateSendPacket(len(data))
	if err != nil {
		return 0, err
	}
	copy(packet, data)
	t.session.SendPacket(packet)
	return len(data), nil
}

func (t *wintunTun) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	if err := windows.SetEvent(t.closeEvent); err != nil {
		log.Printf("wintun: set event: %v", err)
	}
	if t.session != nil {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("wintun: recovered from panic in session.End(): %v", r)
			}
		}()
		t.session.End()
	}
	if err := t.adapter.Close(); err != nil {
		log.Printf("wintun: failed to close adapter: %v", err)
	}
	if err := windows.CloseHandle(t.closeEvent); err != nil {
		log.Printf("wintun: close handle event: %v", err)
	}
	return nil
}
