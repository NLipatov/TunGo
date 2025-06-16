package udp_chacha20

import (
	"bytes"
	"context"
	"io"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

type stubCrypto struct{}

func (stubCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (stubCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

type stubAdapter struct{ writes int32 }

func (a *stubAdapter) Write(p []byte) (int, error) {
	atomic.AddInt32(&a.writes, 1)
	return len(p), nil
}
func (*stubAdapter) Read([]byte) (int, error) { return 0, io.EOF }
func (*stubAdapter) Close() error             { return nil }

type stubMgr struct {
	sess Session
	err  error
}

func (m *stubMgr) Add(Session)                             {}
func (m *stubMgr) Delete(Session)                          {}
func (m *stubMgr) GetByInternalIP([]byte) (Session, error) { return m.sess, m.err }
func (m *stubMgr) GetByExternalIP([]byte) (Session, error) { return m.sess, m.err }

func makeSession(a *stubAdapter) Session {
	return Session{
		connectionAdapter:   a,
		remoteAddrPort:      netip.AddrPort{},
		CryptographyService: stubCrypto{},
		internalIP:          []byte{10, 0, 0, 1},
		externalIP:          []byte{1, 1, 1, 1},
	}
}

// Happy-path: valid IPv4 packet → exactly one write.
func TestTunHandler_HappyPath(t *testing.T) {
	ipHdr := make([]byte, 20)                                // minimal IPv4 header
	ipHdr[0] = 0x45                                          // Version 4, IHL 5
	ipHdr[16], ipHdr[17], ipHdr[18], ipHdr[19] = 10, 0, 0, 1 // dst 10.0.0.1
	buf := bytes.NewBuffer(ipHdr)                            // ← без 12 байт

	a := &stubAdapter{}
	sess := makeSession(a)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := &TunHandler{
		ctx:            ctx,
		reader:         buf,
		sessionManager: &stubMgr{sess: sess},
	}

	done := make(chan error, 1)
	go func() { done <- h.HandleTun() }()

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if n := atomic.LoadInt32(&a.writes); n != 1 {
		t.Fatalf("want 1 write, got %d", n)
	}
}

// Packet <12 bytes must be ignored.
func TestTunHandler_ShortPacketIgnored(t *testing.T) {
	buf := bytes.NewBuffer(make([]byte, 5))
	a := &stubAdapter{}
	sess := makeSession(a)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	h := &TunHandler{
		ctx:            ctx,
		reader:         buf,
		sessionManager: &stubMgr{sess: sess},
	}
	_ = h.HandleTun()

	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Fatalf("short packet must be dropped, writes=%d", n)
	}
}
