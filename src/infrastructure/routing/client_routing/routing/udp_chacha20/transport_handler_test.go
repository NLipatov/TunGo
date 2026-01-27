package udp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"tungo/application/network/rekey"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
)

type dummyRekeyer struct{}

func (dummyRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (dummyRekeyer) SetSendEpoch(uint16)               {}
func (dummyRekeyer) RemoveEpoch(uint16) bool           { return true }

// thTestCrypto implements application.Crypto for testing TransportHandler
// Only Decrypt is used in tests.
type thTestCrypto struct {
	output []byte
	err    error
}

func (m *thTestCrypto) Encrypt([]byte) ([]byte, error) {
	return nil, fmt.Errorf("not used")
}
func (m *thTestCrypto) Decrypt([]byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

type servicePacketMock struct {
}

func (s *servicePacketMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.Unknown, false
}
func (s *servicePacketMock) EncodeLegacy(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}
func (s *servicePacketMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}

type servicePacketSessionResetMock struct {
}

func (s *servicePacketSessionResetMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.SessionReset, true
}
func (s *servicePacketSessionResetMock) EncodeLegacy(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}
func (s *servicePacketSessionResetMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}

// thTestReader simulates a sequence of Read calls for TransportHandler
type thTestReader struct {
	reads []func(p []byte) (int, error)
	idx   int
}

func (r *thTestReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.reads) {
		// block until context done to avoid busy loop
		select {}
	}
	fn := r.reads[r.idx]
	r.idx++
	return fn(p)
}

// thTestWriter captures Write calls and can simulate an error
type thTestWriter struct {
	data [][]byte
	err  error
}

func (w *thTestWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	w.data = append(w.data, buf)
	return len(p), nil
}

func TestHandleTransport_ImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { t.Fatal("Read called despite cancel"); return 0, nil },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, &servicePacketMock{})
	if err := h.HandleTransport(); err != nil {
		t.Errorf("expected nil on immediate cancel, got %v", err)
	}
}

func TestHandleTransport_ReadErrorOther(t *testing.T) {
	errRead := errors.New("read fail")
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { return 0, errRead },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, &thTestCrypto{}, ctrl, &servicePacketMock{})
	exp := fmt.Sprintf("could not read a packet from adapter: %v", errRead)
	if err := h.HandleTransport(); err == nil || err.Error() != exp {
		t.Errorf("expected %q, got %v", exp, err)
	}
}

func TestHandleTransport_ReadDeadlineExceededSkip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { return 0, os.ErrDeadlineExceeded },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, &servicePacketMock{})

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Errorf("expected nil after skip and cancel, got %v", err)
	}
}

func TestHandleTransport_ServerResetSignal(t *testing.T) {
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { p[0] = byte(service.SessionReset); return 1, nil },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, &thTestCrypto{}, ctrl, &servicePacketSessionResetMock{})
	exp := "server requested cryptographyService reset"
	if err := h.HandleTransport(); err == nil || err.Error() != exp {
		t.Errorf("expected %q, got %v", exp, err)
	}
}

func TestHandleTransport_DecryptNonUniqueNonceSkip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1) read 1 byte
	// 2) decrypt returns ErrNonUniqueNonce -> skip
	// 3) next read returns any error after cancel to exit
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { p[0] = 0; return 1, nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{err: chacha20.ErrNonUniqueNonce}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, &servicePacketMock{})

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Errorf("expected nil after nonce skip and cancel, got %v", err)
	}
}

func TestHandleTransport_DecryptErrorFatal(t *testing.T) {
	errDec := errors.New("decrypt fail")
	// reader returns 2 bytes so SessionReset not triggered
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { p[0] = 9; p[1] = 9; return 2, nil },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{err: errDec}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, crypto, ctrl, &servicePacketMock{})
	exp := fmt.Sprintf("failed to decrypt data: %v", errDec)
	if err := h.HandleTransport(); err == nil || err.Error() != exp {
		t.Errorf("expected %q, got %v", exp, err)
	}
}

func TestHandleTransport_WriteError(t *testing.T) {
	d := []byte{9}
	// reader returns data (non-service) -> decrypt -> writer error
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, d); return len(d), nil },
	}}
	errWrite := errors.New("write fail")
	w := &thTestWriter{err: errWrite}
	crypto := &thTestCrypto{output: d}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, crypto, ctrl, &servicePacketMock{})
	exp := fmt.Sprintf("failed to write to TUN: %v", errWrite)
	if err := h.HandleTransport(); err == nil || err.Error() != exp {
		t.Errorf("expected %q, got %v", exp, err)
	}
}

func TestHandleTransport_SuccessThenCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	encrypted := []byte{42}
	decrypted := []byte{100}
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, encrypted); return len(encrypted), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: decrypted}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, &servicePacketMock{})

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("expected nil after success and cancel, got %v", err)
	}

	if len(w.data) != 1 || !bytes.Equal(w.data[0], decrypted) {
		t.Errorf("expected decrypted data %v, got %v", decrypted, w.data)
	}
}
