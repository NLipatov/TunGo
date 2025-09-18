package handshake

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"tungo/application"
	"tungo/domain/network/ip/packet_validation"
)

/***************
 * Test Mocks  *
 ***************/

// mockConn implements application.ConnectionAdapter.
// It simulates controllable short writes/reads and injected errors.
type mockConn struct {
	readBuf        *bytes.Buffer
	writeBuf       *bytes.Buffer
	writeChunk     int // max bytes per Write call; <=0 means no limit
	readChunk      int // max bytes per Read call;  <=0 means no limit
	writeErrAtCall int // if >0, fail on this Write() call index (1-based)
	readErrAtCall  int // if >0, fail on this Read() call index (1-based)
	writeCalls     int
	readCalls      int
}

var _ application.ConnectionAdapter = (*mockConn)(nil)

func newMockConn(readData []byte, writeChunk, readChunk int) *mockConn {
	return &mockConn{
		readBuf:    bytes.NewBuffer(readData),
		writeBuf:   &bytes.Buffer{},
		writeChunk: writeChunk,
		readChunk:  readChunk,
	}
}

func (m *mockConn) Write(p []byte) (int, error) {
	m.writeCalls++
	if m.writeErrAtCall > 0 && m.writeCalls == m.writeErrAtCall {
		return 0, errors.New("mock write error")
	}
	if len(p) == 0 {
		return 0, nil
	}
	n := len(p)
	if m.writeChunk > 0 && m.writeChunk < n {
		n = m.writeChunk
	}
	_, _ = m.writeBuf.Write(p[:n])
	return n, nil
}

func (m *mockConn) Read(p []byte) (int, error) {
	m.readCalls++
	if m.readErrAtCall > 0 && m.readCalls == m.readErrAtCall {
		return 0, io.ErrUnexpectedEOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	if m.readBuf.Len() == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if m.readChunk > 0 && m.readChunk < n {
		n = m.readChunk
	}
	return m.readBuf.Read(p[:n])
}

func (m *mockConn) Close() error { return nil }

/*****************
 * Test Helpers  *
 *****************/

func mustMarshalClientHello(t *testing.T, ch ClientHello) []byte {
	t.Helper()
	b, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	return b
}

/************
 *  Tests   *
 ************/

func TestWriteClientHello_Success(t *testing.T) {
	ch := NewClientHello(
		4,
		net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
		packet_validation.NewDefaultPolicyNewIPValidator(),
	)
	want := mustMarshalClientHello(t, ch)

	under := newMockConn(nil, 0, 0) // no short-write
	clientIO := NewDefaultClientIO(under)

	if err := clientIO.WriteClientHello(ch); err != nil {
		t.Fatalf("WriteClientHello failed: %v", err)
	}
	got := under.writeBuf.Bytes()
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestWriteClientHello_MarshalError(t *testing.T) {
	// Invalid IP version → MarshalBinary must fail
	ch := NewClientHello(0, net.ParseIP("10.0.0.1"), nil, nil, nil, packet_validation.NewDefaultPolicyNewIPValidator())
	under := newMockConn(nil, 0, 0)
	clientIO := NewDefaultClientIO(under)

	if err := clientIO.WriteClientHello(ch); err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if under.writeBuf.Len() != 0 {
		t.Errorf("nothing should be written on marshal error; got %d bytes", under.writeBuf.Len())
	}
}

func TestWriteClientHello_WriteError_Propagated(t *testing.T) {
	ch := NewClientHello(
		4,
		net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
		packet_validation.NewDefaultPolicyNewIPValidator(),
	)
	under := newMockConn(nil, 0, 0)
	under.writeErrAtCall = 1 // fail on the first Write
	clientIO := NewDefaultClientIO(under)

	err := clientIO.WriteClientHello(ch)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "mock write error") {
		t.Errorf("expected raw mock error, got: %v", err)
	}
}

func TestReadServerHello_Success_WithShortReads(t *testing.T) {
	sig := bytes.Repeat([]byte{0xCC}, signatureLength)
	nonce := bytes.Repeat([]byte{0xDD}, nonceLength)
	curve := bytes.Repeat([]byte{0xEE}, curvePublicKeyLength)
	sh := NewServerHello(sig, nonce, curve)
	payload, err := sh.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(ServerHello) failed: %v", err)
	}

	under := newMockConn(payload, 0, 3) // simulate chunked reads
	clientIO := NewDefaultClientIO(under)

	got, err := clientIO.ReadServerHello()
	if err != nil {
		t.Fatalf("ReadServerHello failed: %v", err)
	}
	if !bytes.Equal(got.signature, sig) {
		t.Error("signature mismatch")
	}
	if !bytes.Equal(got.Nonce(), nonce) {
		t.Error("nonce mismatch")
	}
	if !bytes.Equal(got.CurvePublicKey(), curve) {
		t.Error("curve public key mismatch")
	}
}

func TestReadServerHello_ReadError(t *testing.T) {
	N := signatureLength + nonceLength + curvePublicKeyLength
	partial := bytes.Repeat([]byte{0xAA}, N-1) // missing 1 byte

	under := newMockConn(partial, 0, 0)
	clientIO := NewDefaultClientIO(under)

	_, err := clientIO.ReadServerHello()
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read server hello message") {
		t.Errorf("error should be wrapped by client IO, got: %v", err)
	}
}

func TestWriteClientSignature_Success(t *testing.T) {
	s := bytes.Repeat([]byte{0xAB}, signatureLength)
	under := newMockConn(nil, 0, 0) // no short-write
	clientIO := NewDefaultClientIO(under)

	if err := clientIO.WriteClientSignature(NewSignature(s)); err != nil {
		t.Fatalf("WriteClientSignature failed: %v", err)
	}
	if !bytes.Equal(under.writeBuf.Bytes(), s) {
		t.Errorf("signature payload mismatch")
	}
}

func TestWriteClientSignature_MarshalError(t *testing.T) {
	s := bytes.Repeat([]byte{0xAB}, 10) // wrong size
	under := newMockConn(nil, 0, 0)
	clientIO := NewDefaultClientIO(under)

	if err := clientIO.WriteClientSignature(NewSignature(s)); err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if under.writeBuf.Len() != 0 {
		t.Errorf("nothing should be written on marshal error; got %d bytes", under.writeBuf.Len())
	}
}

func TestWriteClientSignature_WriteError_Propagated(t *testing.T) {
	s := bytes.Repeat([]byte{0xAB}, signatureLength)
	under := newMockConn(nil, 0, 0)
	under.writeErrAtCall = 1 // fail on the only Write call
	clientIO := NewDefaultClientIO(under)

	err := clientIO.WriteClientSignature(NewSignature(s))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "mock write error") {
		t.Errorf("expected raw mock error, got: %v", err)
	}
}
