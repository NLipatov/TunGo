package tcp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"tungo/infrastructure/cryptography/chacha20"
)

// MockReader returns a predefined sequence of (data, err), then EOF.
type MockReader struct {
	seq []struct {
		data []byte
		err  error
	}
	i int
}

func (r *MockReader) Read(p []byte) (int, error) {
	if r.i < len(r.seq) {
		e := r.seq[r.i]
		r.i++
		return copy(p, e.data), e.err
	}
	return 0, io.EOF
}

// MockEncoder implements chacha20.TCPEncoder.
type MockEncoder struct {
	Called bool
	Err    error
}

func (e *MockEncoder) Decode(_ []byte, _ *chacha20.TCPPacket) error { return nil }
func (e *MockEncoder) Encode(_ []byte) error {
	e.Called = true
	return e.Err
}

// MockIPParser implements network.IPHeader.
type MockIPParser struct {
	Dest [4]byte
	Err  error
}

func (p *MockIPParser) ParseDestinationAddressBytes(_, dst []byte) error {
	if p.Err != nil {
		return p.Err
	}
	copy(dst, p.Dest[:])
	return nil
}

// mockCryptoService implements application.CryptographyService.
type mockCryptoService struct{ encryptErr error }

func (m *mockCryptoService) Encrypt(b []byte) ([]byte, error) { return b, m.encryptErr }
func (m *mockCryptoService) Decrypt(b []byte) ([]byte, error) { return b, nil }

// mockConn implements net.Conn using bytes.Buffer.
type mockConn struct{ *bytes.Buffer }

func newMockConn() *mockConn                           { return &mockConn{Buffer: &bytes.Buffer{}} }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

// MockErrorConn always errors on Write.
type MockErrorConn struct {
	*mockConn
	Err error
}

func (m *MockErrorConn) Write(_ []byte) (int, error) { return 0, m.Err }

// MockSessionMgr implements session_management.WorkerSessionManager[Session].
type MockSessionMgr struct {
	Sess    Session
	GetErr  error
	Deleted []Session
}

func (m *MockSessionMgr) GetByInternalIP(_ []byte) (Session, error) { return m.Sess, m.GetErr }
func (m *MockSessionMgr) GetByExternalIP(_ []byte) (Session, error) { return m.Sess, m.GetErr }
func (m *MockSessionMgr) Add(_ Session)                             {}
func (m *MockSessionMgr) Delete(s Session)                          { m.Deleted = append(m.Deleted, s) }

// makePacket constructs: 4-byte header + payloadLen + overhead.
func makePacket(payloadLen int) []byte {
	return make([]byte, 4+payloadLen+chacha20poly1305.Overhead)
}

// --- tests ---

func TestHandleTun_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewTunHandler(ctx, &MockReader{}, &MockEncoder{}, &MockIPParser{}, &MockSessionMgr{})
	if err := h.HandleTun(); err != nil {
		t.Errorf("cancelled context: expected nil, got %v", err)
	}
}

func TestHandleTun_EOF(t *testing.T) {
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{nil, nil}}}
	h := NewTunHandler(context.Background(), r, &MockEncoder{}, &MockIPParser{}, &MockSessionMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("EOF: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_ReadErrorRetryThenEOF(t *testing.T) {
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{nil, errors.New("temp")}}}
	h := NewTunHandler(context.Background(), r, &MockEncoder{}, &MockIPParser{}, &MockSessionMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("read error retry: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_PermissionError(t *testing.T) {
	perm := &os.PathError{Op: "read", Err: os.ErrPermission}
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{nil, perm}}}
	h := NewTunHandler(context.Background(), r, &MockEncoder{}, &MockIPParser{}, &MockSessionMgr{})
	if err := h.HandleTun(); !errors.Is(err, os.ErrPermission) {
		t.Errorf("permission error: expected os.ErrPermission, got %v", err)
	}
}

func TestHandleTun_InvalidIPData(t *testing.T) {
	// empty header data => len(data)==0 => skip then EOF
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{[]byte{}, nil}}}
	h := NewTunHandler(context.Background(), r, &MockEncoder{}, &MockIPParser{}, &MockSessionMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("invalid IP data: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_ParserError(t *testing.T) {
	pkt := makePacket(1)
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{pkt, nil}}}
	h := NewTunHandler(context.Background(), r, &MockEncoder{}, &MockIPParser{Err: errors.New("bad hdr")}, &MockSessionMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("parser error: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_SessionNotFound(t *testing.T) {
	pkt := makePacket(1)
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{pkt, nil}}}
	h := NewTunHandler(
		context.Background(),
		r,
		&MockEncoder{},
		&MockIPParser{Dest: [4]byte{1, 2, 3, 4}},
		&MockSessionMgr{GetErr: errors.New("no sess")},
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("session not found: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_EncryptError(t *testing.T) {
	pkt := makePacket(1)
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{pkt, nil}}}
	conn := newMockConn()
	sess := Session{
		conn:                conn,
		CryptographyService: &mockCryptoService{encryptErr: errors.New("enc fail")},
		internalIP:          []byte{5, 5, 5, 5},
		externalIP:          []byte{5, 5, 5, 5},
	}
	h := NewTunHandler(
		context.Background(),
		r,
		&MockEncoder{},
		&MockIPParser{Dest: [4]byte{5, 5, 5, 5}},
		&MockSessionMgr{Sess: sess},
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("encrypt error: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_EncodeError(t *testing.T) {
	pkt := makePacket(1)
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{pkt, nil}}}
	conn := newMockConn()
	sess := Session{
		conn:                conn,
		CryptographyService: &mockCryptoService{},
		internalIP:          []byte{6, 6, 6, 6},
		externalIP:          []byte{6, 6, 6, 6},
	}
	h := NewTunHandler(
		context.Background(),
		r,
		&MockEncoder{Err: errors.New("encode fail")},
		&MockIPParser{Dest: [4]byte{6, 6, 6, 6}},
		&MockSessionMgr{Sess: sess},
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("encode error: expected io.EOF, got %v", err)
	}
}

func TestHandleTun_WriteErrorDeletesSession(t *testing.T) {
	pkt := makePacket(1)
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{pkt, nil}}}
	bad := &MockErrorConn{mockConn: newMockConn(), Err: errors.New("write fail")}
	sess := Session{
		conn:                bad,
		CryptographyService: &mockCryptoService{},
		internalIP:          []byte{7, 7, 7, 7},
		externalIP:          []byte{7, 7, 7, 7},
	}
	mgr := &MockSessionMgr{Sess: sess}
	h := NewTunHandler(
		context.Background(),
		r,
		&MockEncoder{},
		&MockIPParser{Dest: [4]byte{7, 7, 7, 7}},
		mgr,
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Errorf("write error: expected io.EOF, got %v", err)
	}
	if len(mgr.Deleted) != 1 {
		t.Errorf("expected Delete on write error; got %d", len(mgr.Deleted))
	}
}

func TestHandleTun_Success(t *testing.T) {
	pkt := makePacket(1)
	r := &MockReader{seq: []struct {
		data []byte
		err  error
	}{{pkt, nil}}}
	buf := newMockConn()
	sess := Session{
		conn:                buf,
		CryptographyService: &mockCryptoService{},
		internalIP:          []byte{8, 8, 8, 8},
		externalIP:          []byte{8, 8, 8, 8},
	}
	mgr := &MockSessionMgr{Sess: sess}
	enc := &MockEncoder{}
	h := NewTunHandler(
		context.Background(),
		r,
		enc,
		&MockIPParser{Dest: [4]byte{8, 8, 8, 8}},
		mgr,
	)
	err := h.HandleTun()
	if err != io.EOF {
		t.Fatalf("success: expected io.EOF, got %v", err)
	}
	if !enc.Called {
		t.Errorf("success: expected encoder.Encode called")
	}
	// Now the written length should be len(pkt) + 4 (encode-prefix) + overhead
	expected := len(pkt) + 4 + chacha20poly1305.Overhead
	if buf.Len() != expected {
		t.Errorf("success: expected %d bytes written; got %d", expected, buf.Len())
	}
}
