package tun_device

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
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
	event := t.session.ReadWaitEvent()

	for {
		select {
		case <-t.closeCh:
			return 0, errors.New("tun device closed")
		default:
			packet, err := t.session.ReceivePacket()
			if err == nil {
				n := copy(data, packet)
				t.session.ReleaseReceivePacket(packet)
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

func (t *wintunTun) Write(data []byte) (int, error) {
	packet, err := t.session.AllocateSendPacket(len(data))
	if err != nil {
		return 0, err
	}
	copy(packet, data)
	t.session.SendPacket(packet)
	return len(data), nil
}

// Close cleanly shuts down the TUN device.
func (t *wintunTun) Close() error {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return nil
	}
	t.closed = true
	close(t.closeCh)
	t.closeMu.Unlock()

	// Wait for all Read/Write operations to finish
	t.readWg.Wait()

	// End session in safe manner
	if t.session != nil {
		// Recover from driver crash
		defer func() {
			if r := recover(); r != nil {
				log.Printf("wintun:️ Recovered from panic in session.End(): %v", r)
			}
		}()
		t.session.End()
	}

	if err := t.adapter.Close(); err != nil {
		log.Printf("wintun:️ failed to close adapter: %v", err)
	}
	return nil
}
