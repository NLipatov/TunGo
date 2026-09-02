package udp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"tungo/internal/config/settings"
	corechacha20 "tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/rekey"
	chacha20 "tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/keys"
	tunnelrekey "tungo/internal/protocol/rekey"
	"tungo/internal/protocol/servicepacket"

	"golang.org/x/crypto/curve25519"
)

const (
	testRekeyPublicKeyLen = 32
	testRekeyPacketLen    = 3 + testRekeyPublicKeyLen
)

type dummyEpochManager struct{}

func (dummyEpochManager) StageEpoch(_, _ []byte) (uint16, error) { return 0, nil }
func (dummyEpochManager) PromoteSendEpoch(uint16)                {}
func (dummyEpochManager) RetirePreviousEpoch() bool              { return true }

type rekeyAckRecorder struct {
	calls int
}

func (r *rekeyAckRecorder) HandleRekeyAck(uint16, []byte) (bool, error) {
	r.calls++
	return true, nil
}

type testEpochController interface {
	ObservePeerEpoch(uint16)
	ActivateSendEpoch(uint16)
}

type testRekeyAckHandler interface {
	HandleRekeyAck(uint16, []byte) (bool, error)
}

type testTransportRekey struct {
	epoch testEpochController
	ack   testRekeyAckHandler
}

func (r testTransportRekey) ObservePeerEpoch(epoch uint16) {
	if r.epoch != nil {
		r.epoch.ObservePeerEpoch(epoch)
	}
}

func (r testTransportRekey) ActivateSendEpoch(epoch uint16) {
	if r.epoch != nil {
		r.epoch.ActivateSendEpoch(epoch)
	}
}

func (r testTransportRekey) HandleRekeyAck(epoch uint16, packet []byte) (bool, error) {
	if r.ack == nil {
		return false, nil
	}
	return r.ack.HandleRekeyAck(epoch, packet)
}

func newTestTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	crypto crypto,
	epoch testEpochController,
	ack testRekeyAckHandler,
	egress sender,
) *transportHandler {
	if epoch == nil && ack == nil {
		return newTransportHandler(ctx, reader, writer, crypto, nil, egress)
	}
	return newTransportHandler(ctx, reader, writer, crypto, testTransportRekey{epoch: epoch, ack: ack}, egress)
}

func TestHandleControlplane_RekeyAckTypes(t *testing.T) {
	for _, kind := range []servicepacket.HeaderType{servicepacket.RekeyAck, servicepacket.RekeyAckV2} {
		name := "v1"
		if kind == servicepacket.RekeyAckV2 {
			name = "v2"
		}
		t.Run(name, func(t *testing.T) {
			ack := make([]byte, 3)
			if err := servicepacket.Encode(kind, ack); err != nil {
				t.Fatal(err)
			}
			recorder := &rekeyAckRecorder{}
			handler := newTestTransportHandler(context.Background(), nil, nil, nil, nil, recorder, nil)
			handled, err := handler.handleControlplane(0, ack)
			if err != nil || !handled || recorder.calls != 1 {
				t.Fatalf("handled=%v calls=%d err=%v", handled, recorder.calls, err)
			}
		})
	}
}

// thTestCrypto implements config.crypto for testing TransportHandler
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

// thAckCrypto returns payload without the UDP route ID and nonce.
type thAckCrypto struct{}

func (thAckCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (thAckCrypto) Decrypt(b []byte) ([]byte, error) {
	payloadOffset := udpPayloadOffset
	if len(b) <= payloadOffset {
		return nil, fmt.Errorf("cipher too short")
	}
	out := make([]byte, len(b)-payloadOffset)
	copy(out, b[payloadOffset:])
	return out, nil
}

func buildTestUDPPacket(epoch uint16, payload []byte) []byte {
	payloadOffset := udpPayloadOffset
	packet := make([]byte, payloadOffset+len(payload))
	binary.BigEndian.PutUint16(packet[chacha20.EpochOffset:chacha20.EpochOffset+2], epoch)
	copy(packet[payloadOffset:], payload)
	return packet
}

func buildTestRekeyAck(t *testing.T, crypto keys.KeyDeriver) []byte {
	t.Helper()
	serverPub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, testRekeyPacketLen)
	if err := servicepacket.Encode(servicepacket.RekeyAck, payload); err != nil {
		t.Fatal(err)
	}
	copy(payload[3:], serverPub)
	return payload
}

// incEpochManager yields monotonically increasing epochs for rekey tests.
type incEpochManager struct {
	next uint16
	err  error
}

func (r *incEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	if r.err != nil {
		return 0, r.err
	}
	r.next++
	return r.next, nil
}
func (r *incEpochManager) PromoteSendEpoch(uint16)   {}
func (r *incEpochManager) RetirePreviousEpoch() bool { return true }

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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(ctx, r, w, &thTestCrypto{}, ctrl, nil, nil)
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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(context.Background(), r, w, &thTestCrypto{}, ctrl, nil, nil)
	exp := fmt.Sprintf("could not read a packet from adapter: %v", errRead)
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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(ctx, r, w, crypto, ctrl, nil, nil)

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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(context.Background(), r, w, crypto, ctrl, nil, nil)
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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(context.Background(), r, w, crypto, ctrl, nil, nil)
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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(ctx, r, w, crypto, ctrl, nil, nil)

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
	epochManager := &incEpochManager{}
	ctrl := rekey.NewStateMachine(epochManager, []byte("c2s0"), []byte("s2c0"))

	// --- Step 1: fire two RekeyInit sends without ACK in between.
	reader := &fakeReader{readFunc: func(p []byte) (int, error) {
		return 0, nil // no payload needed; just spin the loop
	}}
	writer := &fakeWriter{}
	crypto := &tunhandlerTestRakeCrypto{} // passthrough
	coordinator := tunnelrekey.NewClientRekeyCoordinator(
		&keys.DefaultKeyDeriver{}, ctrl, nil, 5*time.Millisecond, time.Now(),
	)
	tunHandler := newTunHandler(ctx, reader, newPacketSender(writer, crypto), coordinator, nil)

	doneTun := make(chan struct{})
	go func() {
		_ = tunHandler.HandleTun()
		close(doneTun)
	}()

	waitForWrites := func(w *fakeWriter, want int) {
		deadline := time.Now().Add(300 * time.Millisecond)
		for w.packetCount() < want && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitForWrites(writer, 2)
	cancel()  // stop tun handler loop
	<-doneTun // ensure exit
	if len(writer.data) < 2 {
		t.Fatalf("expected at least two RekeyInit packets, got %d", len(writer.data))
	}

	// Extract the retried client public key for expected derivation.
	firstPub := func(pkt []byte) []byte {
		start := udpPayloadOffset + 3
		end := start + testRekeyPublicKeyLen
		if len(pkt) < end {
			t.Fatalf("rekey packet too short: %d", len(pkt))
		}
		out := make([]byte, testRekeyPublicKeyLen)
		copy(out, pkt[start:end])
		return out
	}(writer.data[0])
	secondPub := func(pkt []byte) []byte {
		start := udpPayloadOffset + 3
		end := start + testRekeyPublicKeyLen
		if len(pkt) < end {
			t.Fatalf("rekey packet too short: %d", len(pkt))
		}
		out := make([]byte, testRekeyPublicKeyLen)
		copy(out, pkt[start:end])
		return out
	}(writer.data[1])
	if !bytes.Equal(firstPub, secondPub) {
		t.Fatalf("public keys differ across RekeyInit retries")
	}

	// --- Step 2: craft RekeyAck for the FIRST pubkey and feed TransportHandler.
	hc := &keys.DefaultKeyDeriver{}
	serverPub, serverPriv, err := hc.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to gen server key: %v", err)
	}
	shared, err := curve25519.X25519(serverPriv[:], firstPub)
	if err != nil {
		t.Fatalf("shared derivation failed: %v", err)
	}
	currentC2S, currentS2C := ctrl.CurrentKeys()
	expectedC2S, err := hc.DeriveKey(shared, currentC2S, []byte("tungo-rekey-c2s"))
	if err != nil {
		t.Fatalf("derive c2s failed: %v", err)
	}
	expectedS2C, err := hc.DeriveKey(shared, currentS2C, []byte("tungo-rekey-s2c"))
	if err != nil {
		t.Fatalf("derive s2c failed: %v", err)
	}

	ackPayload := make([]byte, testRekeyPacketLen)
	if err := servicepacket.Encode(servicepacket.RekeyAck, ackPayload); err != nil {
		t.Fatalf("encode ack failed: %v", err)
	}
	copy(ackPayload[3:], serverPub)

	// The server sends the ACK under the epoch that carried the Init.
	cipherAck := buildTestUDPPacket(0, ackPayload)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipherAck); return len(cipherAck), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}

	transportCtx, transportCancel := context.WithCancel(context.Background())
	defer transportCancel()
	h := newTestTransportHandler(transportCtx, r, w, &thAckCrypto{}, ctrl, coordinator, nil)

	errCh := make(chan error, 1)
	go func() { errCh <- h.HandleTransport() }()

	// Wait for rekey to apply.
	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		gotC2S, gotS2C := ctrl.CurrentKeys()
		if bytes.Equal(gotC2S, expectedC2S) && bytes.Equal(gotS2C, expectedS2C) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for rekey apply; c2s=%x s2c=%x", gotC2S, gotS2C)
		}
		time.Sleep(5 * time.Millisecond)
	}
	transportCancel()
	<-errCh

	// Validate derived keys match the ones expected from the ORIGINAL pending priv.
	gotC2S, gotS2C := ctrl.CurrentKeys()
	if !bytes.Equal(gotC2S, expectedC2S) {
		t.Fatalf("C2S key mismatch; got %x want %x", gotC2S, expectedC2S)
	}
	if !bytes.Equal(gotS2C, expectedS2C) {
		t.Fatalf("S2C key mismatch; got %x want %x", gotS2C, expectedS2C)
	}
}

func TestHandleDatagram_RejectsStaleRekeyAckAcrossTransactions(t *testing.T) {
	crypto := &keys.DefaultKeyDeriver{}
	epochManager := &incEpochManager{}
	controller := rekey.NewStateMachine(epochManager, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	coordinator := tunnelrekey.NewClientRekeyCoordinator(crypto, controller, nil, time.Millisecond, now)
	handler := newTestTransportHandler(
		context.Background(),
		&thTestReader{},
		&thTestWriter{},
		&thAckCrypto{},
		controller,
		coordinator,
		nil,
	)

	if _, ok, err := coordinator.MaybeBuildRekeyInit(
		now.Add(time.Second), make([]byte, testRekeyPacketLen),
	); err != nil || !ok {
		t.Fatalf("first init: ok=%v err=%v", ok, err)
	}
	firstAck := buildTestRekeyAck(t, crypto)
	if _, err := handler.handleDatagram(buildTestUDPPacket(0, firstAck)); err != nil {
		t.Fatalf("first ack: %v", err)
	}
	if got := controller.SendEpoch(); got != 1 {
		t.Fatalf("first send epoch = %d, want 1", got)
	}
	if _, err := handler.handleDatagram(buildTestUDPPacket(1, []byte{0xff})); err != nil {
		t.Fatalf("peer confirmation for first rekey: %v", err)
	}

	if _, ok, err := coordinator.MaybeBuildRekeyInit(
		now.Add(2*time.Second), make([]byte, testRekeyPacketLen),
	); err != nil || !ok {
		t.Fatalf("second init: ok=%v err=%v", ok, err)
	}
	if _, err := handler.handleDatagram(buildTestUDPPacket(0, firstAck)); err != nil {
		t.Fatalf("stale first ack: %v", err)
	}
	if got := controller.SendEpoch(); got != 1 {
		t.Fatalf("stale ack changed send epoch to %d, want 1", got)
	}

	secondAck := buildTestRekeyAck(t, crypto)
	if _, err := handler.handleDatagram(buildTestUDPPacket(1, secondAck)); err != nil {
		t.Fatalf("second ack: %v", err)
	}
	if got := controller.SendEpoch(); got != 2 {
		t.Fatalf("second send epoch = %d, want 2", got)
	}
}

// capturingEgress records Send calls for test assertions.
type capturingEgress struct {
	mu      sync.Mutex
	packets [][]byte
	sendErr error
}

func (e *capturingEgress) Send(plaintext []byte) error {
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

func TestCheckLiveness_PingRestartTimeout(t *testing.T) {
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	eg := &capturingEgress{}
	h := newTestTransportHandler(context.Background(), &thTestReader{}, &thTestWriter{}, &thTestCrypto{}, ctrl, nil, eg)
	// Set lastRecvAt far in the past to trigger timeout immediately.
	h.lastRecvAt = time.Now().Add(-settings.PingRestartTimeout - time.Second)

	err := h.checkLiveness()
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("server unreachable")) {
		t.Fatalf("expected 'server unreachable' error, got: %v", err)
	}
}

func TestCheckLiveness_PingSentOnIdle(t *testing.T) {
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	eg := &capturingEgress{}
	h := newTestTransportHandler(context.Background(), &thTestReader{}, &thTestWriter{}, &thTestCrypto{}, ctrl, nil, eg)
	// Set lastRecvAt so that PingInterval is exceeded but PingRestartTimeout is not.
	h.lastRecvAt = time.Now().Add(-settings.PingInterval - time.Second)
	if err := h.checkLiveness(); err != nil {
		t.Fatalf("checkLiveness() error = %v", err)
	}

	pkts := eg.Packets()
	if len(pkts) == 0 {
		t.Fatal("expected at least one Ping sent via egress")
	}
	// Verify the captured packet contains a valid Ping V1 header.
	pkt := pkts[0]
	payload := pkt[udpPayloadOffset:]
	if len(payload) < 3 {
		t.Fatalf("ping packet payload too short: %d", len(payload))
	}
	if payload[0] != servicepacket.Prefix || payload[1] != servicepacket.VersionV1 || payload[2] != byte(servicepacket.Ping) {
		t.Fatalf("unexpected ping payload: %v", payload[:3])
	}
}

func TestHandleDatagram_AuthenticatedPacketResetsLiveness(t *testing.T) {
	encrypted := []byte{0, 42}
	decrypted := []byte{100}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: decrypted}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(context.Background(), &thTestReader{}, w, crypto, ctrl, nil, nil)
	h.lastRecvAt = time.Now().Add(-settings.PingRestartTimeout - time.Second)

	if _, err := h.handleDatagram(encrypted); err != nil {
		t.Fatalf("handleDatagram() error = %v", err)
	}
	if err := h.checkLiveness(); err != nil {
		t.Fatalf("authenticated packet did not reset liveness: %v", err)
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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(ctx, r, w, crypto, ctrl, nil, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil for short packet skip, got %v", err)
	}
}

func TestHandleTransport_EpochExhausted_ReturnsError(t *testing.T) {
	keyDeriver := &keys.DefaultKeyDeriver{}
	serverPub, _, err := keyDeriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}

	rk := &incEpochManager{err: corechacha20.ErrEpochExhausted}
	ctrl := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	coordinator := newDueTestRekeyCoordinator(ctrl)
	if _, ok, buildErr := coordinator.MaybeBuildRekeyInit(
		time.Now(), make([]byte, testRekeyPacketLen),
	); buildErr != nil || !ok {
		t.Fatalf("seed pending rekey: ok=%v err=%v", ok, buildErr)
	}

	// Build a RekeyAck plaintext that will be "decrypted" by thTestCrypto.
	ackPayload := make([]byte, testRekeyPacketLen)
	_ = servicepacket.Encode(servicepacket.RekeyAck, ackPayload)
	copy(ackPayload[3:], serverPub)

	cipher := buildTestUDPPacket(0, ackPayload)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipher); return len(cipher), nil },
	}}
	w := &thTestWriter{}
	crypto := &thAckCrypto{}
	h := newTestTransportHandler(context.Background(), r, w, crypto, ctrl, coordinator, nil)

	err = h.HandleTransport()
	if err == nil {
		t.Fatal("expected epoch exhaustion error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("epoch exhausted")) {
		t.Fatalf("expected 'epoch exhausted' in error, got: %v", err)
	}
}

func TestCheckLiveness_NilEgress_NoIdlePing(t *testing.T) {
	// With nil egress, idle should not attempt to send Ping.
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(context.Background(), &thTestReader{}, &thTestWriter{}, &thTestCrypto{}, ctrl, nil, nil)
	// Set lastRecvAt so PingInterval is exceeded but not PingRestartTimeout.
	h.lastRecvAt = time.Now().Add(-settings.PingInterval - time.Second)

	if err := h.checkLiveness(); err != nil {
		t.Fatalf("checkLiveness() error = %v", err)
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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(ctx, r, w, crypto, ctrl, nil, nil)

	err := h.HandleTransport()
	if err != nil {
		t.Fatalf("expected nil after cancel during decrypt error, got %v", err)
	}
}

func TestHandleTransport_ShortRekeyAck_IgnoredAndContinues(t *testing.T) {
	// A malformed ACK is consumed without terminating the transport loop.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build an ACK with a valid header but no public key.
	ackPayload := make([]byte, 3) // only header, no public key
	_ = servicepacket.Encode(servicepacket.RekeyAck, ackPayload)

	cipher := buildTestUDPPacket(0, ackPayload)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipher); return len(cipher), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, make([]byte, 32), make([]byte, 32))
	coordinator := newTestRekeyCoordinator(ctrl)
	h := newTestTransportHandler(ctx, r, w, &thAckCrypto{}, ctrl, coordinator, nil)

	done := make(chan error)
	go func() { done <- h.HandleTransport() }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("expected nil after malformed ack, got %v", err)
	}
}

func TestCheckLiveness_PingSendError_Swallowed(t *testing.T) {
	// When egress.Send returns an error during Ping, sendPing returns early without panic.
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	eg := &capturingEgress{sendErr: errors.New("send failed")}
	h := newTestTransportHandler(context.Background(), &thTestReader{}, &thTestWriter{}, &thTestCrypto{}, ctrl, nil, eg)
	h.lastRecvAt = time.Now().Add(-settings.PingInterval - time.Second)

	if err := h.checkLiveness(); err != nil {
		t.Fatalf("checkLiveness() error = %v", err)
	}
}

func TestHandleDatagram_TooShortPacket_Ignored(t *testing.T) {
	h := newTestTransportHandler(context.Background(), &thTestReader{}, &thTestWriter{}, &thTestCrypto{}, nil, nil, nil)
	n, err := h.handleDatagram([]byte{0x01})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 written bytes, got %d", n)
	}
}

func TestHandleDatagram_WriteErrorAfterCancel_IsSuppressed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := newTestTransportHandler(ctx, &thTestReader{}, &thTestWriter{err: errors.New("write fail")}, &thTestCrypto{output: []byte{1, 2}}, nil, nil, nil)
	n, err := h.handleDatagram([]byte{0x00, 0x01})
	if err != nil {
		t.Fatalf("expected nil error after cancel, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 written bytes, got %d", n)
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
	h := newTestTransportHandler(ctx, r, w, crypto, nil, nil, nil)

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

	pongSP := []byte{0xFF, 0x01, byte(servicepacket.Pong)}
	cipher := buildTestUDPPacket(0, pongSP)
	r := &thTestReader{reads: []func(p []byte) (int, error){
		func(p []byte) (int, error) { copy(p, cipher); return len(cipher), nil },
		func(p []byte) (int, error) { <-ctx.Done(); return 0, errors.New("stop") },
	}}
	w := &thTestWriter{}
	crypto := &thTestCrypto{output: pongSP}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTestTransportHandler(ctx, r, w, crypto, ctrl, nil, nil)

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
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))

	h := newTestTransportHandler(ctx, r, w, crypto, ctrl, nil, nil)
	// Force context done before write error check.
	cancel()

	err := h.HandleTransport()
	// With ctx cancelled, the write error should be suppressed.
	if err != nil {
		t.Fatalf("expected nil after cancel, got %v", err)
	}
}
