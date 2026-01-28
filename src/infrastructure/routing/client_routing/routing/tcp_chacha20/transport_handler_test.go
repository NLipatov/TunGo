package tcp_chacha20

import (
	"context"
	"errors"
	"io"
	"testing"
	"tungo/application/network/rekey"
	"tungo/domain/network/service"

	"golang.org/x/crypto/chacha20poly1305"
)

/* ─── Mocks (prefixed with the struct under test: TransportHandler*) ─── */

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

type servicePacketMock struct{}

func (servicePacketMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.Unknown, false
}
func (servicePacketMock) EncodeLegacy(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}
func (servicePacketMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}

/* ─── Tests ─── */

func TestTransportHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx, rdr(), io.Discard, &TransportHandlerMockCrypto{}, ctrl, servicePacketMock{})
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTransportHandler_ReadError(t *testing.T) {
	readErr := errors.New("read fail")
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, servicePacketMock{},
	)
	if err := h.HandleTransport(); !errors.Is(err, readErr) {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestTransportHandler_ReadErrorAfterCancel_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(ctx,
		rdr(struct {
			data []byte
			err  error
		}{nil, errors.New("any")}),
		io.Discard,
		&TransportHandlerMockCrypto{}, ctrl, servicePacketMock{},
	)
	if err := h.HandleTransport(); err != nil {
		t.Fatalf("want nil when ctx canceled, got %v", err)
	}
}

func TestTransportHandler_InvalidTooShort_ThenEOF(t *testing.T) {
	short := make([]byte, chacha20poly1305.Overhead-1) // triggers "invalid length"
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
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
		&TransportHandlerMockCrypto{}, ctrl, servicePacketMock{},
	)
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF after invalid short frame, got %v", err)
	}
}

func TestTransportHandler_DecryptError(t *testing.T) {
	cipher := make([]byte, chacha20poly1305.Overhead+8)
	decErr := errors.New("decrypt fail")
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		io.Discard,
		&TransportHandlerMockCrypto{decErr: decErr}, ctrl, servicePacketMock{},
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

	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTransportHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{cipher, nil}),
		w,
		&TransportHandlerMockCrypto{decOut: plain}, ctrl, servicePacketMock{},
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

	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
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
		&TransportHandlerMockCrypto{decOut: plain}, ctrl, servicePacketMock{},
	)
	if err := h.HandleTransport(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}
