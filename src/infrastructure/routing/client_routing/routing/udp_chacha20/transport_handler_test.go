package udp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
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

// thAckCrypto returns payload without the leading epoch bytes.
type thAckCrypto struct{}

func (thAckCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (thAckCrypto) Decrypt(b []byte) ([]byte, error) {
	if len(b) <= 2 {
		return nil, fmt.Errorf("cipher too short")
	}
	out := make([]byte, len(b)-2)
	copy(out, b[2:])
	return out, nil
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

// incRekeyer yields monotonically increasing epochs for rekey tests.
type incRekeyer struct {
	next uint16
}

func (r *incRekeyer) Rekey(_, _ []byte) (uint16, error) {
	r.next++
	return r.next, nil
}
func (r *incRekeyer) SetSendEpoch(uint16)     {}
func (r *incRekeyer) RemoveEpoch(uint16) bool { return true }

// thTestReader simulates a sequence of Read calls for TransportHandler
type thTestReader struct {
	reads []func(p []byte) (int, error)
	idx   int
}

func (r *thTestReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.reads) {
		return 0, io.EOF
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
	d := []byte{0, 9} // epoch + payload
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, d); return len(d), nil },
	}}
	errWrite := errors.New("write fail")
	w := &thTestWriter{err: errWrite}
	crypto := &thTestCrypto{output: d[1:]} // decrypted payload
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

	encrypted := []byte{0, 42} // epoch + payload
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

// Regression test for repeated RekeyInit before Ack: pending private key must stay the same,
// otherwise the RekeyAck computed with the first pubkey would derive mismatched session keys.
func TestHandleTransport_RekeyAckAfterDoubleInit_UsesOriginalPendingKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Shared controller for TunHandler and TransportHandler.
	rekeyer := &incRekeyer{}
	ctrl := rekey.NewController(rekeyer, []byte("c2s0"), []byte("s2c0"), false)

	// --- Step 1: fire two RekeyInit sends without ACK in between.
	reader := &fakeReader{readFunc: func(p []byte) (int, error) {
		return 0, nil // no payload needed; just spin the loop
	}}
	writer := &fakeWriter{}
	crypto := &tunhandlerTestRakeCrypto{} // passthrough
	tunHandler := NewTunHandler(ctx, reader, writer, crypto, ctrl, service.NewDefaultPacketHandler()).(*TunHandler)
	tunHandler.rekeyInterval = 5 * time.Millisecond
	tunHandler.rotateAt = time.Now().UTC().Add(tunHandler.rekeyInterval)

	doneTun := make(chan struct{})
	go func() {
		_ = tunHandler.HandleTun()
		close(doneTun)
	}()

	waitForWrites := func(w *fakeWriter, want int) {
		deadline := time.Now().Add(300 * time.Millisecond)
		for len(w.data) < want && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitForWrites(writer, 2)
	cancel()  // stop tun handler loop
	<-doneTun // ensure exit
	if len(writer.data) < 2 {
		t.Fatalf("expected at least two RekeyInit packets, got %d", len(writer.data))
	}

	// Extract pending priv and first public key for expected derivation.
	pendingPriv, ok := ctrl.PendingRekeyPrivateKey()
	if !ok {
		t.Fatal("pending priv key missing")
	}
	firstPub := func(pkt []byte) []byte {
		start := chacha20poly1305.NonceSize + 3
		end := start + service.RekeyPublicKeyLen
		if len(pkt) < end {
			t.Fatalf("rekey packet too short: %d", len(pkt))
		}
		out := make([]byte, service.RekeyPublicKeyLen)
		copy(out, pkt[start:end])
		return out
	}(writer.data[0])
	secondPub := func(pkt []byte) []byte {
		start := chacha20poly1305.NonceSize + 3
		end := start + service.RekeyPublicKeyLen
		if len(pkt) < end {
			t.Fatalf("rekey packet too short: %d", len(pkt))
		}
		out := make([]byte, service.RekeyPublicKeyLen)
		copy(out, pkt[start:end])
		return out
	}(writer.data[1])
	if !bytes.Equal(firstPub, secondPub) {
		t.Fatalf("public keys differ across RekeyInit retries")
	}

	// --- Step 2: craft RekeyAck for the FIRST pubkey and feed TransportHandler.
	hc := &handshake.DefaultCrypto{}
	serverPub, _, err := hc.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to gen server key: %v", err)
	}
	shared, err := curve25519.X25519(pendingPriv[:], serverPub)
	if err != nil {
		t.Fatalf("shared derivation failed: %v", err)
	}
	expectedC2S, err := hc.DeriveKey(shared, ctrl.CurrentClientToServerKey(), []byte("tungo-rekey-c2s"))
	if err != nil {
		t.Fatalf("derive c2s failed: %v", err)
	}
	expectedS2C, err := hc.DeriveKey(shared, ctrl.CurrentServerToClientKey(), []byte("tungo-rekey-s2c"))
	if err != nil {
		t.Fatalf("derive s2c failed: %v", err)
	}

	ackPayload := make([]byte, service.RekeyPacketLen)
	if _, err := service.NewDefaultPacketHandler().EncodeV1(service.RekeyAck, ackPayload); err != nil {
		t.Fatalf("encode ack failed: %v", err)
	}
	copy(ackPayload[3:], serverPub)

	// Ciphertext: epoch bytes + plaintext (Decrypt stub will strip the epoch).
	cipherAck := append([]byte{0, 42}, ackPayload...)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipherAck); return len(cipherAck), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}

	transportCtx, transportCancel := context.WithCancel(context.Background())
	defer transportCancel()
	h := NewTransportHandler(transportCtx, r, w, &thAckCrypto{}, ctrl, service.NewDefaultPacketHandler()).(*TransportHandler)
	h.handshakeCrypto = hc

	errCh := make(chan error, 1)
	go func() { errCh <- h.HandleTransport() }()

	// Wait for rekey to apply.
	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		if ctrl.State() == rekey.StateStable && ctrl.LastRekeyEpoch == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for rekey apply; state=%v epoch=%d", ctrl.State(), ctrl.LastRekeyEpoch)
		}
		time.Sleep(5 * time.Millisecond)
	}
	transportCancel()
	_ = <-errCh

	// Validate derived keys match the ones expected from the ORIGINAL pending priv.
	if got := ctrl.CurrentClientToServerKey(); !bytes.Equal(got, expectedC2S) {
		t.Fatalf("C2S key mismatch; got %x want %x", got, expectedC2S)
	}
	if got := ctrl.CurrentServerToClientKey(); !bytes.Equal(got, expectedS2C) {
		t.Fatalf("S2C key mismatch; got %x want %x", got, expectedS2C)
	}
	if _, ok := ctrl.PendingRekeyPrivateKey(); ok {
		t.Fatalf("pending priv should be cleared after ack")
	}
}
