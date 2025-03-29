package tun_device

import (
	"errors"
	"golang.org/x/sys/windows"
	"log"
	"sync"
)

// wintunTun is a Windows-specific TUN device using the wintun driver (https://www.wintun.net).
type wintunTun struct {
	adapter wintun.Adapter
	session *wintun.Session
	name    string
	mtu     int
	closeCh chan struct{}
	readWg  sync.WaitGroup
	closeMu sync.Mutex
	closed  bool
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
	windows.SetEvent(t.closeEvent)

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
	windows.CloseHandle(t.closeEvent)
	return nil
}
