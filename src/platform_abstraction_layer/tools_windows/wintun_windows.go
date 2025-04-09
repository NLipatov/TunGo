package pal_windows

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"log"
	"tungo/application"
)

type wintunTun struct {
	adapter    wintun.Adapter
	session    wintun.Session
	closeEvent windows.Handle
	closed     bool
}

func NewWinTun(adapter wintun.Adapter, session wintun.Session) application.TunDevice {
	handle, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		log.Println("Error creating winTun handle:", err)
	}
	return &wintunTun{
		adapter:    adapter,
		session:    session,
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
			var timeout uint32 = 250
			ret, waitErr := windows.WaitForSingleObject(event, timeout)
			if ret == windows.WAIT_FAILED {
				return 0, fmt.Errorf("session closed")
			}
			if waitErr != nil {
				return 0, waitErr
			}
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

	_ = windows.SetEvent(t.closeEvent)

	t.session.End()
	_ = t.adapter.Close()
	_ = windows.CloseHandle(t.closeEvent)
	return nil
}
