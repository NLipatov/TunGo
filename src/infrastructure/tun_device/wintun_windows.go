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

// Read reads a packet from the TUN interface.
func (t *wintunTun) Read(data []byte) (int, error) {
	t.readWg.Add(1)
	defer t.readWg.Done()

	event := t.session.ReadWaitEvent()
	timeout := uint32(500)

	for {
		select {
		case <-t.closeCh:
			return 0, errors.New("tun device closed")
		default:
			status, _ := windows.WaitForSingleObject(event, timeout)
			if status != windows.WAIT_OBJECT_0 {
				continue
			}

			select {
			case <-t.closeCh:
				return 0, errors.New("tun device closed")
			default:
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
	}
}

// Write sends a packet to the TUN interface.
func (t *wintunTun) Write(data []byte) (int, error) {
	t.readWg.Add(1)
	defer t.readWg.Done()

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
