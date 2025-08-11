package handshake

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"tungo/infrastructure/network/tcp/adapters"

	"tungo/infrastructure/network"
)

// DefaultClientIOMockConnShortRW simulates an underlying transport that can short-write/short-read
// and inject errors on specific calls. It is intentionally "dumb" (no framing).
type DefaultClientIOMockConnShortRW struct {
	readBuf        *bytes.Buffer
	writeBuf       *bytes.Buffer
	writeChunk     int // max bytes per Write call; <=0 means no limit
	readChunk      int // max bytes per Read call; <=0 means no limit
	writeErrAtCall int // if >0, fail on this Write() call index (1-based)
	readErrAtCall  int // if >0, fail on this Read() call index (1-based)
	writeCalls     int
	readCalls      int
}

func newDefaultClientIOMockConnShortRW(readData []byte, writeChunk, readChunk int) *DefaultClientIOMockConnShortRW {
	return &DefaultClientIOMockConnShortRW{
		readBuf:    bytes.NewBuffer(readData),
		writeBuf:   &bytes.Buffer{},
		writeChunk: writeChunk,
		readChunk:  readChunk,
	}
}

func (m *DefaultClientIOMockConnShortRW) Write(p []byte) (int, error) {
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

func (m *DefaultClientIOMockConnShortRW) Read(p []byte) (int, error) {
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

func (m *DefaultClientIOMockConnShortRW) Close() error { return nil }

// frame helpers

func readFrameFromUnderlying(t *testing.T, under *DefaultClientIOMockConnShortRW) (hdr uint16, payload []byte) {
	t.Helper()
	raw := under.writeBuf.Bytes()
	if len(raw) < 2 {
		t.Fatalf("too short frame: %x", raw)
	}
	hdr = binary.BigEndian.Uint16(raw[:2])
	payload = raw[2:]
	return
}

func mustMarshalClientHello(t *testing.T, ch ClientHello) []byte {
	t.Helper()
	b, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	return b
}

// --- Tests ---

func TestWriteClientHello_Success_ViaAdapterWithShortWrites(t *testing.T) {
	// Build a valid ClientHello
	ch := NewClientHello(
		4,
		net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
		network.NewDefaultPolicyNewIPValidator(),
	)
	wantPayload := mustMarshalClientHello(t, ch)

	// Underlying transport short-writes in chunks of 5 bytes.
	under := newDefaultClientIOMockConnShortRW(nil, 5, 0)
	conn := adapters.NewTcpAdapter(under) // guarantees full frame write
	clientIO := NewDefaultClientIO(conn)

	if err := clientIO.WriteClientHello(ch); err != nil {
		t.Fatalf("WriteClientHello failed: %v", err)
	}

	gotLen, gotPayload := readFrameFromUnderlying(t, under)
	if int(gotLen) != len(wantPayload) {
		t.Fatalf("frame length = %d; want %d", gotLen, len(wantPayload))
	}
	if !bytes.Equal(gotPayload, wantPayload) {
		t.Errorf("payload mismatch:\n got  %x\n want %x", gotPayload, wantPayload)
	}
}

func TestWriteClientHello_MarshalError(t *testing.T) {
	// Invalid IP version => MarshalBinary should fail before any write.
	ch := NewClientHello(
		0,
		net.ParseIP("10.0.0.1"),
		nil, nil, nil,
		network.NewDefaultPolicyNewIPValidator(),
	)
	under := newDefaultClientIOMockConnShortRW(nil, 0, 0)
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	if err := clientIO.WriteClientHello(ch); err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if under.writeBuf.Len() != 0 {
		t.Errorf("nothing should be written on marshal error; got %d bytes", under.writeBuf.Len())
	}
}

func TestWriteClientHello_WriteError_OnHeader(t *testing.T) {
	ch := NewClientHello(
		4,
		net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
		network.NewDefaultPolicyNewIPValidator(),
	)
	under := newDefaultClientIOMockConnShortRW(nil, 5, 0)
	under.writeErrAtCall = 1 // fail when adapter writes the 2-byte header
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	err := clientIO.WriteClientHello(ch)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "write length prefix") {
		t.Errorf("error should mention prefix write, got: %v", err)
	}
}

func TestWriteClientHello_WriteError_OnPayload(t *testing.T) {
	ch := NewClientHello(
		4,
		net.ParseIP("10.0.0.1"),
		bytes.Repeat([]byte{1}, ed25519.PublicKeySize),
		bytes.Repeat([]byte{2}, curvePublicKeyLength),
		bytes.Repeat([]byte{3}, nonceLength),
		network.NewDefaultPolicyNewIPValidator(),
	)
	under := newDefaultClientIOMockConnShortRW(nil, 5, 0)
	under.writeErrAtCall = 3 // 1: header, 2+: payload attempts → fail during payload
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	if err := clientIO.WriteClientHello(ch); err == nil {
		t.Fatal("expected write error during payload, got nil")
	}
}

func TestReadServerHello_Success_ViaAdapterWithShortReads(t *testing.T) {
	// Prepare a valid ServerHello frame.
	sig := bytes.Repeat([]byte{0xCC}, signatureLength)
	nonce := bytes.Repeat([]byte{0xDD}, nonceLength)
	curve := bytes.Repeat([]byte{0xEE}, curvePublicKeyLength)
	sh := NewServerHello(sig, nonce, curve)
	payload, err := sh.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(ServerHello) failed: %v", err)
	}
	var framed bytes.Buffer
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	framed.Write(hdr[:])
	framed.Write(payload)

	// Underlying transport short-reads (e.g., 3 bytes at a time).
	under := newDefaultClientIOMockConnShortRW(framed.Bytes(), 0, 3)
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

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

func TestReadServerHello_ReadError_OnHeader(t *testing.T) {
	// Provide less than 2 bytes so adapter fails on reading length prefix.
	under := newDefaultClientIOMockConnShortRW([]byte{0x01}, 0, 0)
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	_, err := clientIO.ReadServerHello()
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read server hello message") {
		t.Errorf("error should be wrapped by client IO, got: %v", err)
	}
}

func TestReadServerHello_ReadError_OnPayload(t *testing.T) {
	// Frame says N bytes, but only N-1 available → adapter.Read should error on payload.
	N := signatureLength + nonceLength + curvePublicKeyLength
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(N))
	var framed bytes.Buffer
	framed.Write(hdr[:])
	framed.Write(bytes.Repeat([]byte{0xAA}, N-1)) // one byte short

	under := newDefaultClientIOMockConnShortRW(framed.Bytes(), 0, 0)
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	_, err := clientIO.ReadServerHello()
	if err == nil {
		t.Fatal("expected read payload error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read server hello message") {
		t.Errorf("error should be wrapped by client IO, got: %v", err)
	}
}

func TestWriteClientSignature_Success_ViaAdapterWithShortWrites(t *testing.T) {
	s := bytes.Repeat([]byte{0xAB}, signatureLength)
	under := newDefaultClientIOMockConnShortRW(nil, 7, 0) // short-write in chunks of 7
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	if err := clientIO.WriteClientSignature(NewSignature(s)); err != nil {
		t.Fatalf("WriteClientSignature failed: %v", err)
	}
	hdr, payload := readFrameFromUnderlying(t, under)
	if int(hdr) != len(s) {
		t.Fatalf("frame length = %d; want %d", hdr, len(s))
	}
	if !bytes.Equal(payload, s) {
		t.Errorf("signature payload mismatch:\n got  %x\n want %x", payload, s)
	}
}

func TestWriteClientSignature_MarshalError(t *testing.T) {
	// Wrong signature size → MarshalBinary error.
	s := bytes.Repeat([]byte{0xAB}, 10)
	under := newDefaultClientIOMockConnShortRW(nil, 0, 0)
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	if err := clientIO.WriteClientSignature(NewSignature(s)); err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if under.writeBuf.Len() != 0 {
		t.Errorf("nothing should be written on marshal error; got %d bytes", under.writeBuf.Len())
	}
}

func TestWriteClientSignature_WriteError_OnHeader(t *testing.T) {
	s := bytes.Repeat([]byte{0xAB}, signatureLength)
	under := newDefaultClientIOMockConnShortRW(nil, 10, 0)
	under.writeErrAtCall = 1 // fail on header write
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	err := clientIO.WriteClientSignature(NewSignature(s))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "write length prefix") {
		t.Errorf("error should mention prefix write, got: %v", err)
	}
}

func TestWriteClientSignature_WriteError_OnPayload(t *testing.T) {
	s := bytes.Repeat([]byte{0xAB}, signatureLength)
	under := newDefaultClientIOMockConnShortRW(nil, 10, 0)
	under.writeErrAtCall = 3 // fail during payload write
	conn := adapters.NewTcpAdapter(under)
	clientIO := NewDefaultClientIO(conn)

	if err := clientIO.WriteClientSignature(NewSignature(s)); err == nil {
		t.Fatal("expected write error during payload, got nil")
	}
}
