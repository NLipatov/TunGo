package tun_device

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"log"
	"tungo/application"
)

// wintunTun is a Windows-specific TUN device using the wintun driver (https://www.wintun.net).
type wintunTun struct {
	adapter    wintun.Adapter
	session    *wintun.Session
	name       string
	mtu        int
	closeEvent windows.Handle
	closed     bool
}

func newWinTun(adapter wintun.Adapter, session wintun.Session, name string, mtu int) application.TunDevice {
	handle, handleErr := windows.CreateEvent(nil, 0, 0, nil)
	if handleErr != nil {
		log.Println("Error creating winTun handle:", handleErr)
	}

	return &wintunTun{
		adapter:    adapter,
		session:    &session,
		name:       name,
		mtu:        mtu,
		closeEvent: handle,
	}
}

func (t *wintunTun) Read(data []byte) (int, error) {
	handles := []windows.Handle{t.session.ReadWaitEvent(), t.closeEvent}
	for {
		ret, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			return 0, err
		}
		if ret == windows.WAIT_OBJECT_0+1 {
			return 0, errors.New("tun device closed")
		}
		packet, err := t.session.ReceivePacket()
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				continue
			}
			return 0, err
		}
		n := copy(data, packet)
		t.session.ReleaseReceivePacket(packet)
		return n, nil
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
	setEventErr := windows.SetEvent(t.closeEvent)
	if setEventErr != nil {
		log.Printf("wintun: set event: %v", setEventErr)
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
	closeHandleEventErr := windows.CloseHandle(t.closeEvent)
	if closeHandleEventErr != nil {
		log.Printf("wintun: close handle event: %v", closeHandleEventErr)
	}

	return nil
}
