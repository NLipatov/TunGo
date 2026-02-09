package tcp_chacha20

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
	"tungo/infrastructure/cryptography/chacha20/rekey"
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

type dummyRekeyer struct{}

func (dummyRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (dummyRekeyer) SetSendEpoch(uint16)               {}
func (dummyRekeyer) RemoveEpoch(uint16) bool           { return true }

/* --- Tests --- */

func TestTransportHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, rdr(), io.Discard, &TransportHandlerMockCrypto{}, ctrl, nil)
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTransportHandler_ReadError(t *testing.T) {
	readErr := errors.New("read fail")
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil,
	)
	if err := h.HandleTransport(); !errors.Is(err, readErr) {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestTransportHandler_ReadErrorAfterCancel_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx,
		rdr(struct {
			data []byte
			err  error
		}{nil, errors.New("any")}),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, nil,
	)
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil when ctx canceled, got %v", err)
	}
}

func TestTransportHandler_InvalidTooShort_ThenEOF(t *testing.T) {
	short := make([]byte, chacha20poly1305.Overhead-1) // triggers "invalid length"
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
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
		&TransportHandlerMockCrypto{}, ctrl, nil,
	)
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after invalid short frame, got %v", err)
	}
}

func TestTransportHandler_DecryptError(t *testing.T) {
	cipher := make([]byte, chacha20poly1305.Overhead+8)
	decErr := errors.New("decrypt fail")
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decErr: decErr}, ctrl, nil,
	)
	if err := h.HandleTransport(); !errors.Is(err, decErr) {
		t.Fatalf("want decrypt error, got %v", err)
	}
}

func TestTransportHandler_WriteError(t *testing.T) {
	cipher := make([]byte, chacha20poly1305.Overhead+4)
	wErr := errors.New("write fail")
	w := &TransportHandlerMockWriter{err: wErr}
	plain := []byte{1, 2, 3, 4}

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		w,
		&TransportHandlerMockCrypto{decOut: plain}, ctrl, nil,
	)
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

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{cipher, nil}, // one decrypted packet
			struct {
				data []byte
				err  error
			}{nil, io.EOF}, // then EOF
		),
		w,
		&TransportHandlerMockCrypto{decOut: plain}, ctrl, nil,
	)
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

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
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
		&TransportHandlerMockCrypto{decOut: ackPayload}, ctrl, nil,
	)
	// RekeyAck is consumed; handler continues to next read which is EOF.
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after RekeyAck, got %v", err)
	}
}

func TestTransportHandler_RekeyAck_NilController(t *testing.T) {
	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(ackPayload))

	h := NewTransportHandler(context.Background(),
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
	)
	// With nil controller, handleRekeyAck returns immediately; handler continues to EOF.
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestTransportHandler_TCPDecryptErrorAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decErr := errors.New("decrypt fail")
	cipher := make([]byte, chacha20poly1305.Overhead+8)
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx,
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decErr: decErr}, ctrl, nil,
	)
	// ctx already canceled -> decrypt error is suppressed, returns nil.
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil when ctx canceled, got %v", err)
	}
}

func TestTransportHandler_EpochExhausted_ReturnsError(t *testing.T) {
	epochPayload := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.EpochExhausted, epochPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(epochPayload))

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: epochPayload}, ctrl, nil,
	)
	err := h.HandleTransport()
	if !errors.Is(err, ErrEpochExhausted) {
		t.Fatalf("want ErrEpochExhausted, got %v", err)
	}
}

func TestTransportHandler_HandleRekeyAck_EpochExhausted(t *testing.T) {
	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(ackPayload))

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	ctrl.LastRekeyEpoch = 65001 // >= 65000 triggers epoch exhaustion

	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decOut: ackPayload}, ctrl, nil,
	)
	err := h.HandleTransport()
	if !errors.Is(err, ErrEpochExhausted) {
		t.Fatalf("want ErrEpochExhausted on epoch exhaustion, got %v", err)
	}
}

type TransportHandlerMockEgress struct {
	mu    sync.Mutex
	pings [][]byte
	err   error
}

func (e *TransportHandlerMockEgress) SendDataIP(_ []byte) error { return nil }
func (e *TransportHandlerMockEgress) SendControl(p []byte) error {
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)

	h := NewTransportHandler(context.Background(),
		rdr(), io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, egress,
	)
	impl := h.(*TransportHandler)
	impl.sendPing()

	if egress.pingCount() != 1 {
		t.Fatalf("expected 1 ping sent, got %d", egress.pingCount())
	}
}

func TestTransportHandler_SendPing_EgressError(t *testing.T) {
	egress := &TransportHandlerMockEgress{err: errors.New("send fail")}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)

	h := NewTransportHandler(context.Background(),
		rdr(), io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, egress,
	)
	impl := h.(*TransportHandler)
	// Should not panic
	impl.sendPing()
}

func TestTransportHandler_KeepaliveLoop_CancelStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	egress := &TransportHandlerMockEgress{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)

	h := NewTransportHandler(ctx,
		rdr(), io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, egress,
	)
	impl := h.(*TransportHandler)

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

func TestTransportHandler_Pong_Consumed(t *testing.T) {
	pongPayload := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.Pong, pongPayload)

	cipher := make([]byte, chacha20poly1305.Overhead+len(pongPayload))

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
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
		&TransportHandlerMockCrypto{decOut: pongPayload}, ctrl, nil,
	)
	// Pong is consumed silently; handler continues to next read which is EOF.
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after Pong, got %v", err)
	}
}
