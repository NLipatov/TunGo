package tcp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/chacha20poly1305"
)

/* --- Mocks (prefixed with the struct under test: TransportHandler*) --- */

type TransportHandlerMockWriter struct {
	writes int
	err    error
}

func (w *TransportHandlerMockWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.err != nil {
		return 0, w.err
	}
	return len(p), nil
}

type TransportHandlerMockCrypto struct {
	decOut []byte
	decErr error
}

func (m *TransportHandlerMockCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (m *TransportHandlerMockCrypto) Decrypt(_ []byte) ([]byte, error) {
	return m.decOut, m.decErr
}

type TransportHandlerMockRekeyAck struct {
	calls int
}

func (m *TransportHandlerMockRekeyAck) HandleRekeyAck(uint16, []byte) (bool, error) {
	m.calls++
	return true, nil
}

type dummyEpochManager struct {
	epoch uint16
	err   error
}

func (d dummyEpochManager) StageEpoch(_, _ []byte) (uint16, error) { return d.epoch, d.err }
func (dummyEpochManager) PromoteSendEpoch(uint16)                  {}
func (dummyEpochManager) RetirePreviousEpoch() bool                { return true }

/* --- Tests --- */

func TestTransportHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(ctx, rdr(), io.Discard, &TransportHandlerMockCrypto{}, ctrl, nil, nil)
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTransportHandler_ReadError(t *testing.T) {
	readErr := errors.New("read fail")
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil, nil)

	if err := h.HandleTransport(); !errors.Is(err, readErr) {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestTransportHandler_ReadErrorAfterCancel_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(ctx,
		rdr(struct {
			data []byte
			err  error
		}{nil, errors.New("any")}),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil, nil)

	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil when ctx canceled, got %v", err)
	}
}

func TestTransportHandler_InvalidTooShort_ThenEOF(t *testing.T) {
	short := make([]byte, chacha20poly1305.Overhead-1) // triggers "invalid length"
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{short, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil, nil)

	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after invalid short frame, got %v", err)
	}
}

func TestTransportHandler_DecryptError(t *testing.T) {
	cipher := make([]byte, chacha20poly1305.Overhead+8)
	decErr := errors.New("decrypt fail")
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decErr: decErr}, ctrl, nil, nil)

	if err := h.HandleTransport(); !errors.Is(err, decErr) {
		t.Fatalf("want decrypt error, got %v", err)
	}
}

func TestTransportHandler_WriteError(t *testing.T) {
	cipher := make([]byte, chacha20poly1305.Overhead+4)
	wErr := errors.New("write fail")
	w := &TransportHandlerMockWriter{err: wErr}
	plain := []byte{1, 2, 3, 4}

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		w,
		&TransportHandlerMockCrypto{decOut: plain}, ctrl, nil, nil)

	if err := h.HandleTransport(); !errors.Is(err, wErr) {
		t.Fatalf("want write error, got %v", err)
	}
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}

func TestTransportHandler_Happy_ThenEOF(t *testing.T) {
	cipher := make([]byte, chacha20poly1305.Overhead+6)
	w := &TransportHandlerMockWriter{}
	plain := []byte{9, 9, 9, 9, 9, 9}

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{cipher, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		w,
		&TransportHandlerMockCrypto{decOut: plain}, ctrl, nil, nil)

	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}

func TestTransportHandler_RekeyAck_Handled(t *testing.T) {
	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(ackPayload))

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	rekeyAck := &TransportHandlerMockRekeyAck{}
	h := newTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{cipher, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: ackPayload}, ctrl, rekeyAck, nil)

	// RekeyAck is consumed; handler continues to next read which is EOF.
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after RekeyAck, got %v", err)
	}
	if rekeyAck.calls != 1 {
		t.Fatalf("rekey ack calls=%d, want 1", rekeyAck.calls)
	}
}

func TestTransportHandler_RekeyAck_NilHandler(t *testing.T) {
	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(ackPayload))

	h := newTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{cipher, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: ackPayload}, nil, nil,

		nil)

	// With no ACK handler, the control packet is consumed and processing continues.
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestTransportHandler_TCPDecryptErrorAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decErr := errors.New("decrypt fail")
	cipher := make([]byte, chacha20poly1305.Overhead+8)
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(ctx,
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decErr: decErr}, ctrl, nil, nil)

	// ctx already canceled -> decrypt error is suppressed, returns nil.
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil when ctx canceled, got %v", err)
	}
}

func TestTransportHandler_EpochExhausted_ReturnsError(t *testing.T) {
	epochPayload := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.EpochExhausted, epochPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(epochPayload))

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: epochPayload}, ctrl, nil, nil)

	err := h.HandleTransport()
	if !errors.Is(err, errEpochExhausted) {
		t.Fatalf("want ErrEpochExhausted, got %v", err)
	}
}

func TestTransportHandler_HandleRekeyAck_EpochExhausted(t *testing.T) {
	keyDeriver := &primitives.DefaultKeyDeriver{}
	serverPub, _, err := keyDeriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}

	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)
	copy(ackPayload[3:], serverPub)

	cipher := make([]byte, chacha20poly1305.Overhead+len(ackPayload))

	ctrl := rekey.NewStateMachine(dummyEpochManager{err: chacha20.ErrEpochExhausted}, []byte("c2s"), []byte("s2c"))
	coordinator := newDueTestRekeyCoordinator(ctrl)
	if _, ok, buildErr := coordinator.MaybeBuildRekeyInit(
		time.Now(), make([]byte, service_packet.RekeyPacketLen),
	); buildErr != nil || !ok {
		t.Fatalf("seed pending rekey: ok=%v err=%v", ok, buildErr)
	}

	h := newTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: ackPayload}, ctrl, coordinator,

		nil)

	err = h.HandleTransport()
	if !errors.Is(err, errEpochExhausted) {
		t.Fatalf("want ErrEpochExhausted on epoch exhaustion, got %v", err)
	}
}

type TransportHandlerMockEgress struct {
	mu    sync.Mutex
	pings [][]byte
	err   error
}

func (e *TransportHandlerMockEgress) Send(p []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pings = append(e.pings, append([]byte(nil), p...))
	return e.err
}
func (e *TransportHandlerMockEgress) Close() error { return nil }

func (e *TransportHandlerMockEgress) pingCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.pings)
}

func TestTransportHandler_SendPing_Success(t *testing.T) {
	egress := &TransportHandlerMockEgress{}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))

	h := newTransportHandler(context.Background(),
		rdr(), io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil,

		egress)

	impl := h
	impl.sendPing()

	if egress.pingCount() != 1 {
		t.Fatalf("expected 1 ping sent, got %d", egress.pingCount())
	}
}

func TestTransportHandler_SendPing_EgressError(t *testing.T) {
	egress := &TransportHandlerMockEgress{err: errors.New("send fail")}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))

	h := newTransportHandler(context.Background(),
		rdr(), io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil,

		egress)

	impl := h
	// Should not panic
	impl.sendPing()
}

func TestTransportHandler_NewTransportHandler_InitializesRecvTime(t *testing.T) {
	h := newTransportHandler(context.Background(), rdr(), io.Discard, &TransportHandlerMockCrypto{}, nil, nil, nil)
	impl := h
	if got := impl.lastRecvNano.Load(); got == 0 {
		t.Fatal("expected initial receive timestamp to be set")
	}
}

func TestTransportHandler_HandleRekeyAck_ErrorDoesNotBubble(t *testing.T) {
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, make([]byte, 32), make([]byte, 32))
	h := newTransportHandler(context.Background(), rdr(), io.Discard, &TransportHandlerMockCrypto{}, ctrl, nil, nil)
	impl := h

	shortAck := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, shortAck)
	if err := impl.handleRekeyAck(0, shortAck); err != nil {
		t.Fatalf("expected nil on ack install/apply error, got %v", err)
	}
}

func TestTransportHandler_KeepaliveLoop_CancelStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	egress := &TransportHandlerMockEgress{}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))

	h := newTransportHandler(ctx,
		rdr(), io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil,

		egress)

	impl := h

	done := make(chan struct{})
	go func() {
		impl.keepaliveLoop()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepaliveLoop did not stop after cancel")
	}
}

func TestTransportHandler_InvalidTooLong_ThenEOF(t *testing.T) {
	long := make([]byte, 1500+18+1) // DefaultEthernetMTU + TCPChacha20Overhead + 1
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{long, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil, nil)

	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after invalid long frame, got %v", err)
	}
}

// blockingReader blocks until ctx is canceled, then returns an error.
// This simulates a real TCP read that's interrupted by context cancellation.
type blockingReader struct {
	ctx context.Context
}

func (r *blockingReader) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func TestTransportHandler_ReadError_ContextCanceledDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(ctx,
		&blockingReader{ctx: ctx},
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil, nil)

	done := make(chan error, 1)
	go func() { done <- h.HandleTransport() }()

	// Give goroutine time to enter Read, then cancel
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want nil (ctx canceled during read), got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HandleTransport to return")
	}
}

func TestTransportHandler_Pong_Consumed(t *testing.T) {
	pongPayload := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.Pong, pongPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(pongPayload))

	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	h := newTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{cipher, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: pongPayload}, ctrl, nil, nil)

	// Pong is consumed silently; handler continues to next read which is EOF.
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after Pong, got %v", err)
	}
}
