package udp_chacha20

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"os"
	"sync/atomic"
	"testing"
)

type testUdpReader struct {
	seq []struct {
		data []byte
		err  error
	}
	i int
}

func (r *testUdpReader) Read(p []byte) (int, error) {
	if r.i < len(r.seq) {
		rec := r.seq[r.i]
		r.i++
		n := copy(p, rec.data)
		return n, rec.err
	}
	return 0, io.EOF
}

type testParser struct {
	wantDst [4]byte
	retErr  error
}

func (p *testParser) ParseDestinationAddressBytes(_, dst []byte) error {
	if p.retErr != nil {
		return p.retErr
	}
	copy(dst, p.wantDst[:])
	return nil
}

type testCrypto struct {
	encryptErr error
}

func (c *testCrypto) Encrypt(b []byte) ([]byte, error) { return b, c.encryptErr }
func (c *testCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

type testAdapter struct {
	writes   int32
	writeErr error
}

func (a *testAdapter) Write(_ []byte) (int, error) {
	atomic.AddInt32(&a.writes, 1)
	return 1, a.writeErr
}
func (a *testAdapter) Read([]byte) (int, error) { return 0, io.EOF }
func (a *testAdapter) Close() error             { return nil }

type testMgr struct {
	sess Session
	err  error
}

func (m *testMgr) Add(Session)                              {}
func (m *testMgr) Delete(Session)                           {}
func (m *testMgr) GetByInternalIP([4]byte) (Session, error) { return m.sess, m.err }
func (m *testMgr) GetByExternalIP([4]byte) (Session, error) { return m.sess, m.err }

func makeSession(a *testAdapter, c *testCrypto) Session {
	return Session{
		connectionAdapter:   a,
		remoteAddrPort:      netip.AddrPort{},
		CryptographyService: c,
		internalIP:          [4]byte{10, 0, 0, 1},
		externalIP:          [4]byte{1, 1, 1, 1},
	}
}

func TestTunHandler_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewTunHandler(ctx, &testUdpReader{}, &testParser{}, &testMgr{sess: makeSession(&testAdapter{}, &testCrypto{})})
	if err := h.HandleTun(); err != nil {
		t.Errorf("context cancel: want nil, got %v", err)
	}
}

func TestTunHandler_EOF(t *testing.T) {
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{make([]byte, 20), io.EOF}}}
	h := NewTunHandler(context.Background(), r, &testParser{}, &testMgr{sess: makeSession(&testAdapter{}, &testCrypto{})})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("EOF: want io.EOF, got %v", err)
	}
}

func TestTunHandler_ShortPacket(t *testing.T) {
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{make([]byte, 5), nil}, {make([]byte, 20), io.EOF}}}
	a := &testAdapter{}
	h := NewTunHandler(context.Background(), r, &testParser{}, &testMgr{sess: makeSession(a, &testCrypto{})})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("short packet: want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Errorf("short packet: writes=%d, want 0", n)
	}
}

func TestTunHandler_ParserError(t *testing.T) {
	hdr := make([]byte, 20)
	hdr[0] = 0x45
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{append(make([]byte, 12), hdr...), nil}, {nil, io.EOF}}}
	a := &testAdapter{}
	parser := &testParser{retErr: errors.New("bad")}
	h := NewTunHandler(context.Background(), r, parser, &testMgr{sess: makeSession(a, &testCrypto{})})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("parser error: want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Errorf("parser error: writes=%d, want 0", n)
	}
}

func TestTunHandler_SessionNotFound(t *testing.T) {
	hdr := make([]byte, 20)
	hdr[0] = 0x45
	hdr[16], hdr[17], hdr[18], hdr[19] = 10, 0, 0, 1
	frame := append(make([]byte, 12), hdr...)
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{frame, nil}, {nil, io.EOF}}}
	a := &testAdapter{}
	parser := &testParser{wantDst: [4]byte{10, 0, 0, 1}}
	mgr := &testMgr{sess: makeSession(a, &testCrypto{}), err: errors.New("no sess")}
	h := NewTunHandler(context.Background(), r, parser, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("session not found: want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Errorf("session not found: writes=%d, want 0", n)
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	hdr := make([]byte, 20)
	hdr[0] = 0x45
	hdr[16], hdr[17], hdr[18], hdr[19] = 10, 0, 0, 1
	frame := append(make([]byte, 12), hdr...)
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{frame, nil}, {nil, io.EOF}}}
	a := &testAdapter{}
	parser := &testParser{wantDst: [4]byte{10, 0, 0, 1}}
	crypto := &testCrypto{encryptErr: errors.New("encrypt fail")}
	mgr := &testMgr{sess: makeSession(a, crypto)}
	h := NewTunHandler(context.Background(), r, parser, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("encrypt error: want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Errorf("encrypt error: writes=%d, want 0", n)
	}
}

func TestTunHandler_WriteError(t *testing.T) {
	hdr := make([]byte, 20)
	hdr[0] = 0x45
	hdr[16], hdr[17], hdr[18], hdr[19] = 10, 0, 0, 1
	frame := append(make([]byte, 12), hdr...)
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{frame, nil}, {nil, io.EOF}}}
	a := &testAdapter{writeErr: errors.New("write fail")}
	parser := &testParser{wantDst: [4]byte{10, 0, 0, 1}}
	mgr := &testMgr{sess: makeSession(a, &testCrypto{})}
	h := NewTunHandler(context.Background(), r, parser, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("write error: want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 1 {
		t.Errorf("write error: writes=%d, want 1", n)
	}
}

func TestTunHandler_ReadOsNotExist(t *testing.T) {
	notExist := &os.PathError{Op: "read", Path: "/dev/net/tun", Err: os.ErrNotExist}
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{nil, notExist}}}
	h := NewTunHandler(context.Background(), r, &testParser{}, &testMgr{sess: makeSession(&testAdapter{}, &testCrypto{})})
	if err := h.HandleTun(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.IsNotExist: want os.ErrNotExist, got %v", err)
	}
}

func TestTunHandler_ReadOsPermission(t *testing.T) {
	perm := &os.PathError{Op: "read", Path: "/dev/net/tun", Err: os.ErrPermission}
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{nil, perm}}}
	h := NewTunHandler(context.Background(), r, &testParser{}, &testMgr{sess: makeSession(&testAdapter{}, &testCrypto{})})
	if err := h.HandleTun(); !errors.Is(err, os.ErrPermission) {
		t.Errorf("os.IsPermission: want os.ErrPermission, got %v", err)
	}
}

func TestTunHandler_HappyPath(t *testing.T) {
	hdr := make([]byte, 20)
	hdr[0] = 0x45
	hdr[16], hdr[17], hdr[18], hdr[19] = 10, 0, 0, 1
	frame := append(make([]byte, 12), hdr...)
	r := &testUdpReader{seq: []struct {
		data []byte
		err  error
	}{{frame, nil}, {nil, io.EOF}}}
	a := &testAdapter{}
	parser := &testParser{wantDst: [4]byte{10, 0, 0, 1}}
	mgr := &testMgr{sess: makeSession(a, &testCrypto{})}
	h := NewTunHandler(context.Background(), r, parser, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("happy: want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 1 {
		t.Errorf("happy: writes=%d, want 1", n)
	}
}
