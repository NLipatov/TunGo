package udp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

type dummyRekeyer struct{}

func (dummyRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (dummyRekeyer) SetSendEpoch(uint16)               {}
func (dummyRekeyer) RemoveEpoch(uint16) bool           { return true }

// thTestCrypto implements application.crypto for testing TransportHandler
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, nil)
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, &thTestCrypto{}, ctrl, nil)
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Errorf("expected nil after skip and cancel, got %v", err)
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Errorf("expected nil after nonce skip and cancel, got %v", err)
	}
}

func TestHandleTransport_DecryptErrorDropped(t *testing.T) {
	errDec := errors.New("decrypt fail")
	// reader returns bad packet, then EOF
	// decrypt error should be dropped, not terminate session
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { p[0] = 9; p[1] = 9; return 2, nil },
		func(p []byte) (int, error) { return 0, io.EOF },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{err: errDec}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, crypto, ctrl, nil)
	// Should exit with read error (EOF), not decrypt error
	err := h.HandleTransport()
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Errorf("expected EOF error after dropped decrypt, got %v", err)
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(), r, w, crypto, ctrl, nil)
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil)

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
	ctrl := rekey.NewStateMachine(rekeyer, []byte("c2s0"), []byte("s2c0"), false)

	// --- Step 1: fire two RekeyInit sends without ACK in between.
	reader := &fakeReader{readFunc: func(p []byte) (int, error) {
		return 0, nil // no payload needed; just spin the loop
	}}
	writer := &fakeWriter{}
	crypto := &tunhandlerTestRakeCrypto{} // passthrough
	tunHandler := NewTunHandler(ctx, reader, connection.NewDefaultEgress(writer, crypto), ctrl, nil).(*TunHandler)
	tunHandler.rekeyInit.SetInterval(5 * time.Millisecond)
	tunHandler.rekeyInit.SetRotateAt(time.Now().UTC().Add(tunHandler.rekeyInit.Interval()))

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
		end := start + service_packet.RekeyPublicKeyLen
		if len(pkt) < end {
			t.Fatalf("rekey packet too short: %d", len(pkt))
		}
		out := make([]byte, service_packet.RekeyPublicKeyLen)
		copy(out, pkt[start:end])
		return out
	}(writer.data[0])
	secondPub := func(pkt []byte) []byte {
		start := chacha20poly1305.NonceSize + 3
		end := start + service_packet.RekeyPublicKeyLen
		if len(pkt) < end {
			t.Fatalf("rekey packet too short: %d", len(pkt))
		}
		out := make([]byte, service_packet.RekeyPublicKeyLen)
		copy(out, pkt[start:end])
		return out
	}(writer.data[1])
	if !bytes.Equal(firstPub, secondPub) {
		t.Fatalf("public keys differ across RekeyInit retries")
	}

	// --- Step 2: craft RekeyAck for the FIRST pubkey and feed TransportHandler.
	hc := &primitives.DefaultKeyDeriver{}
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

	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	if _, err := service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload); err != nil {
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
	h := NewTransportHandler(transportCtx, r, w, &thAckCrypto{}, ctrl, nil).(*TransportHandler)
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

// capturingEgress records SendControl calls for test assertions.
type capturingEgress struct {
	mu      sync.Mutex
	packets [][]byte
	sendErr error
}

func (e *capturingEgress) SendDataIP(plaintext []byte) error {
	return e.send(plaintext)
}

func (e *capturingEgress) SendControl(plaintext []byte) error {
	return e.send(plaintext)
}

func (e *capturingEgress) send(plaintext []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sendErr != nil {
		return e.sendErr
	}
	buf := make([]byte, len(plaintext))
	copy(buf, plaintext)
	e.packets = append(e.packets, buf)
	return nil
}

func (e *capturingEgress) Close() error { return nil }

func (e *capturingEgress) Packets() [][]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([][]byte, len(e.packets))
	copy(out, e.packets)
	return out
}

func TestHandleTransport_PingRestartTimeout(t *testing.T) {
	// Reader returns only deadline-exceeded errors; after PingRestartTimeout
	// the handler must return an error indicating unreachable server.
	r := &thTestReader{reads: make([]func(p []byte) (int, error), 0)}
	// Fill enough reads to outlast the timeout. Each deadline-exceeded wakes ~immediately in test.
	for i := 0; i < 200; i++ {
		r.reads = append(r.reads, func(p []byte) (int, error) {
			return 0, os.ErrDeadlineExceeded
		})
	}
	w := &thTestWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	eg := &capturingEgress{}
	h := NewTransportHandler(context.Background(), r, w, &thTestCrypto{}, ctrl, eg).(*TransportHandler)
	// Set lastRecvAt far in the past to trigger timeout immediately.
	h.lastRecvAt = time.Now().Add(-settings.PingRestartTimeout - time.Second)

	err := h.HandleTransport()
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("server unreachable")) {
		t.Fatalf("expected 'server unreachable' error, got: %v", err)
	}
}

func TestHandleTransport_PingSentOnIdle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First read: deadline exceeded (triggers Ping send).
	// Second read: blocks until cancel.
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { return 0, os.ErrDeadlineExceeded },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	eg := &capturingEgress{}
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, eg).(*TransportHandler)
	// Set lastRecvAt so that PingInterval is exceeded but PingRestartTimeout is not.
	h.lastRecvAt = time.Now().Add(-settings.PingInterval - time.Second)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	pkts := eg.Packets()
	if len(pkts) == 0 {
		t.Fatal("expected at least one Ping sent via egress")
	}
	// Verify the captured packet contains a valid Ping V1 header.
	pkt := pkts[0]
	payload := pkt[chacha20poly1305.NonceSize:]
	if len(payload) < 3 {
		t.Fatalf("ping packet payload too short: %d", len(payload))
	}
	if payload[0] != service_packet.Prefix || payload[1] != service_packet.VersionV1 || payload[2] != byte(service_packet.Ping) {
		t.Fatalf("unexpected ping payload: %v", payload[:3])
	}
}

func TestHandleTransport_RecvResetsPingTimer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Sequence:
	// 1. Read deadline exceeded (idle, but within PingRestartTimeout)
	// 2. Read data (successful decrypt resets lastRecvAt)
	// 3. Read deadline exceeded again — should NOT timeout
	// 4. Block until cancel
	encrypted := []byte{0, 42}
	decrypted := []byte{100}
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { return 0, os.ErrDeadlineExceeded },
		func(p []byte) (int, error) { copy(p, encrypted); return len(encrypted), nil },
		func(p []byte) (int, error) { return 0, os.ErrDeadlineExceeded },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: decrypted}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("expected nil (recv should have reset timer), got %v", err)
	}
}

func TestHandleTransport_ShortPacket_SkippedAfterServiceCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A 1-byte packet with len<2 — should be silently skipped.
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { p[0] = 0x45; return 1, nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: []byte{42}} // should not be reached
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil for short packet skip, got %v", err)
	}
}

func TestHandleTransport_EpochExhausted_ReturnsError(t *testing.T) {
	// When rekeyController.LastRekeyEpoch >= 65000 and a RekeyAck arrives,
	// handleControlplane should return an error about epoch exhaustion.
	rk := &incRekeyer{}
	ctrl := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)

	// Force LastRekeyEpoch to exhausted state.
	ctrl.LastRekeyEpoch = 65001

	// Build a RekeyAck plaintext that will be "decrypted" by thTestCrypto.
	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)

	// Ciphertext: 2 bytes epoch + ack payload.
	cipher := append([]byte{0, 0}, ackPayload...)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipher); return len(cipher), nil },
	}}
	w := &thTestWriter{}
	crypto := &thAckCrypto{}
	h := NewTransportHandler(context.Background(), r, w, crypto, ctrl, nil)

	err := h.HandleTransport()
	if err == nil {
		t.Fatal("expected epoch exhaustion error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("epoch exhausted")) {
		t.Fatalf("expected 'epoch exhausted' in error, got: %v", err)
	}
}

func TestHandleTransport_NilEgress_NoIdlePing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// With nil egress, idle should not attempt to send Ping.
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { return 0, os.ErrDeadlineExceeded },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, nil).(*TransportHandler)
	// Set lastRecvAt so PingInterval is exceeded but not PingRestartTimeout.
	h.lastRecvAt = time.Now().Add(-settings.PingInterval - time.Second)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	// No panic = success (nil egress handled gracefully).
}

func TestHandleTransport_DecryptErrorAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errDec := errors.New("decrypt fail")
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) {
			cancel() // cancel before decrypt processes
			p[0] = 9
			p[1] = 9
			return 2, nil
		},
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{err: errDec}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil)

	err := h.HandleTransport()
	if err != nil {
		t.Fatalf("expected nil after cancel during decrypt error, got %v", err)
	}
}

func TestHandleTransport_RekeyAckInstallError_LoggedAndContinues(t *testing.T) {
	// When ClientHandleRekeyAck returns an error (e.g., short ack packet),
	// the handler should log and continue (not return the error).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build a short RekeyAck that won't have enough data for ClientHandleRekeyAck.
	ackPayload := make([]byte, 3) // only header, no public key
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)

	cipher := append([]byte{0, 0}, ackPayload...)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipher); return len(cipher), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, make([]byte, 32), make([]byte, 32), false)
	h := NewTransportHandler(ctx, r, w, &thAckCrypto{}, ctrl, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("expected nil (ack error should be logged, not returned), got %v", err)
	}
}

func TestHandleTransport_PingSendError_Swallowed(t *testing.T) {
	// When egress.SendControl returns an error during Ping, sendPing returns early without panic.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { return 0, os.ErrDeadlineExceeded },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	eg := &capturingEgress{sendErr: errors.New("send failed")}
	h := NewTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, eg).(*TransportHandler)
	h.lastRecvAt = time.Now().Add(-settings.PingInterval - time.Second)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHandleTransport_NilRekeyController(t *testing.T) {
	// With nil rekeyController, handleDatagram should skip epoch/rekey logic.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	encrypted := []byte{0, 42}
	decrypted := []byte{100}
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, encrypted); return len(encrypted), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: decrypted}
	h := NewTransportHandler(ctx, r, w, crypto, nil, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(w.data) != 1 || !bytes.Equal(w.data[0], decrypted) {
		t.Fatalf("expected decrypted data written, got %v", w.data)
	}
}

func TestHandleTransport_EncryptedPong_ConsumedSilently(t *testing.T) {
	// When decrypted data is a Pong service packet, handleControlplane should
	// return handled=true, err=nil (default case), and no TUN write occurs.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pongSP := []byte{0xFF, 0x01, byte(service_packet.Pong)}
	cipher := append([]byte{0, 0}, pongSP...) // epoch prefix + payload
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipher); return len(cipher), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: pongSP}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	// Pong should be consumed; no TUN write.
	if len(w.data) != 0 {
		t.Fatalf("expected no TUN writes for Pong, got %d", len(w.data))
	}
}

func TestHandleTransport_WriteErrorAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	encrypted := []byte{0, 42}
	decrypted := []byte{100}
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, encrypted); return len(encrypted), nil },
	}}
	errWrite := errors.New("write fail")
	w := &thTestWriter{err: errWrite}
	crypto := &thTestCrypto{output: decrypted}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)

	h := NewTransportHandler(ctx, r, w, crypto, ctrl, nil).(*TransportHandler)
	// Force context done before write error check.
	cancel()

	err := h.HandleTransport()
	// With ctx cancelled, the write error should be suppressed.
	if err != nil {
		t.Fatalf("expected nil after cancel, got %v", err)
	}
}
