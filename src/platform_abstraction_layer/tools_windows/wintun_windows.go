package pal_windows

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"log"
	"sync"
	"sync/atomic"
	"tungo/application"
)

type wintunTun struct {
	adapter     wintun.Adapter
	session     atomic.Pointer[wintun.Session]
	closeEvent  windows.Handle
	closed      atomic.Bool
	reopenMutex sync.Mutex
}

func NewWinTun(adapter wintun.Adapter, session wintun.Session) application.TunDevice {
	handle, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		log.Println("Error creating winTun handle:", err)
	}
	tun := &wintunTun{
		adapter:    adapter,
		closeEvent: handle,
	}
	tun.session.Store(&session)
	return tun
}

func (d *wintunTun) reopenSession() error {
	d.reopenMutex.Lock()
	defer d.reopenMutex.Unlock()

	if d.closed.Load() {
		return fmt.Errorf("device already closed")
	}

	oldSession := d.session.Load()
	if oldSession != nil {
		oldSession.End()
	}

	newSession, err := d.adapter.StartSession(0x800000)
	if err != nil {
		return err
	}

	ptr := new(wintun.Session)
	*ptr = newSession
	d.session.Store(ptr)
	return nil
}

func (d *wintunTun) Read(data []byte) (int, error) {
	for {
		if d.closed.Load() {
			return 0, fmt.Errorf("session closed")
		}

		session := d.session.Load()
		packet, err := session.ReceivePacket()
		if err == nil {
			n := copy(data, packet)
			session.ReleaseReceivePacket(packet)
			return n, nil
		}

		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			event := session.ReadWaitEvent()
			timeout := uint32(250)
			ret, waitErr := windows.WaitForSingleObject(event, timeout)
			if ret == windows.WAIT_FAILED {
				return 0, fmt.Errorf("session closed")
			}
			if waitErr != nil {
				return 0, waitErr
			}
			continue
		}

		if errors.Is(err, windows.ERROR_HANDLE_EOF) {
			if reopenErr := d.reopenSession(); reopenErr != nil {
				return 0, reopenErr
			}
			continue
		}

		return 0, err
	}
}

func (d *wintunTun) Write(data []byte) (int, error) {
	for {
		if d.closed.Load() {
			return 0, fmt.Errorf("session closed")
		}

		session := d.session.Load()
		packet, err := session.AllocateSendPacket(len(data))
		if err != nil {
			if err.Error() == "Reached the end of the file." {
				if reopenErr := d.reopenSession(); reopenErr != nil {
					return 0, reopenErr
				}
				continue
			}
			return 0, err
		}

		copy(packet, data)
		session.SendPacket(packet)
		return len(data), nil
	}
}

func (d *wintunTun) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	if err := windows.SetEvent(d.closeEvent); err != nil {
		log.Printf("failed to signal close event: %v", err)
	}

	session := d.session.Load()
	if session != nil {
		session.End()
	}

	_ = d.adapter.Close()
	_ = windows.CloseHandle(d.closeEvent)
	return nil
}
