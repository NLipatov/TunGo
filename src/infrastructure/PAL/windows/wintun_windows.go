package windows

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
	adapter     *wintun.Adapter
	session     *wintun.Session
	sessionMu   sync.RWMutex
	reopenMutex sync.Mutex
	closeEvent  windows.Handle
	closed      atomic.Bool
}

func NewWinTun(adapter *wintun.Adapter) (application.TunDevice, error) {
	handle, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating winTun handle: %w", err)
	}

	session, err := adapter.StartSession(0x800000)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("session start error: %w", err)
	}

	sessionPtr := new(wintun.Session)
	*sessionPtr = session

	return &wintunTun{
		adapter:    adapter,
		session:    sessionPtr,
		closeEvent: handle,
	}, nil
}

func (d *wintunTun) reopenSession() error {
	d.reopenMutex.Lock()
	defer d.reopenMutex.Unlock()

	if d.closed.Load() {
		return fmt.Errorf("device already closed")
	}

	d.sessionMu.Lock()
	defer d.sessionMu.Unlock()

	if d.session != nil {
		d.session.End()
	}

	newSession, err := d.adapter.StartSession(0x800000) // 8MB ring buffer
	if err != nil {
		return err
	}
	sessionPtr := new(wintun.Session)
	*sessionPtr = newSession
	d.session = sessionPtr
	return nil
}

func (d *wintunTun) Read(data []byte) (int, error) {
	for {
		if d.closed.Load() {
			return 0, fmt.Errorf("device closed")
		}

		d.sessionMu.RLock()
		session := d.session
		d.sessionMu.RUnlock()

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
			if ret == windows.WAIT_FAILED || waitErr != nil {
				return 0, fmt.Errorf("session closed")
			}
			continue
		}

		if errors.Is(err, windows.ERROR_HANDLE_EOF) {
			if err := d.reopenSession(); err != nil {
				return 0, err
			}
			continue
		}

		return 0, err
	}
}

func (d *wintunTun) Write(data []byte) (int, error) {
	for {
		if d.closed.Load() {
			return 0, fmt.Errorf("device closed")
		}

		d.sessionMu.RLock()
		session := d.session
		packet, err := session.AllocateSendPacket(len(data))
		d.sessionMu.RUnlock()

		if err != nil {
			if errors.Is(err, windows.ERROR_HANDLE_EOF) {
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

	d.sessionMu.Lock()
	if d.session != nil {
		d.session.End()
		d.session = nil
	}
	d.sessionMu.Unlock()

	if err := d.adapter.Close(); err != nil {
		log.Printf("failed to close adapter: %v", err)
	}
	if err := windows.CloseHandle(d.closeEvent); err != nil {
		log.Printf("failed to close event handle: %v", err)
	}
	return nil
}
