package tcp_chacha20

import (
	"context"
	"errors"
	"io"
	"testing"
)

// ---- Mocks (prefixed with TunHandler*) ----

type TunHandlerMockReader struct {
	seq []struct {
		data []byte
		err  error
	}
	i int
}

func (r *TunHandlerMockReader) Read(p []byte) (int, error) {
	if r.i >= len(r.seq) {
		return 0, io.EOF
	}
	rec := r.seq[r.i]
	r.i++
	n := copy(p, rec.data)
	return n, rec.err
}

type TunHandlerMockWriter struct {
	writes int
	err    error
}

func (w *TunHandlerMockWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.err != nil {
		return 0, w.err
	}
	return len(p), nil
}

type TunHandlerMockCrypto struct{ err error }

func (m *TunHandlerMockCrypto) Encrypt(b []byte) ([]byte, error) {
	return append([]byte(nil), b...), m.err
}
func (m *TunHandlerMockCrypto) Decrypt([]byte) ([]byte, error) { return nil, nil }

// helper
func rdr(seq ...struct {
	data []byte
	err  error
}) *TunHandlerMockReader {
	return &TunHandlerMockReader{seq: seq}
}

// ---- Tests ----

func TestTunHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before entering the loop

	h := NewTunHandler(ctx, rdr(), io.Discard, &TunHandlerMockCrypto{})
	if err := h.HandleTun(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTunHandler_EOF(t *testing.T) {
	h := NewTunHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, io.EOF}),
		io.Discard,
		&TunHandlerMockCrypto{},
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestTunHandler_ReadError(t *testing.T) {
	readErr := errors.New("read fail")
	h := NewTunHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}),
		io.Discard,
		&TunHandlerMockCrypto{},
	)
	if err := h.HandleTun(); !errors.Is(err, readErr) {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	encErr := errors.New("encrypt fail")
	h := NewTunHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{1, 2, 3}, nil},
		),
		io.Discard,
		&TunHandlerMockCrypto{err: encErr},
	)
	if err := h.HandleTun(); !errors.Is(err, encErr) {
		t.Fatalf("want encrypt error, got %v", err)
	}
}

func TestTunHandler_WriteError(t *testing.T) {
	wErr := errors.New("write fail")
	w := &TunHandlerMockWriter{err: wErr}
	h := NewTunHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{[]byte{9, 9}, nil}),
		w,
		&TunHandlerMockCrypto{},
	)
	if err := h.HandleTun(); !errors.Is(err, wErr) {
		t.Fatalf("want write error, got %v", err)
	}
	// writer should be called exactly once
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}

func TestTunHandler_HappyPath_SinglePacket_ThenEOF(t *testing.T) {
	w := &TunHandlerMockWriter{}
	h := NewTunHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{0xAA, 0xBB}, nil}, // one payload
			struct {
				data []byte
				err  error
			}{nil, io.EOF}, // exit
		),
		w,
		&TunHandlerMockCrypto{},
	)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}

func TestTunHandler_ReadError_WhenContextCanceled_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // context already canceled before read

	readErr := errors.New("any read error")
	h := NewTunHandler(
		ctx,
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}), // reader returns an error
		io.Discard,
		&TunHandlerMockCrypto{},
	)

	// When ctx is canceled, the read error path should return nil.
	if err := h.HandleTun(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTunHandler_ZeroLengthPayload_ThenEOF(t *testing.T) {
	w := &TunHandlerMockWriter{}
	h := NewTunHandler(
		context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{}, nil}, // zero-length read, still processed
			struct {
				data []byte
				err  error
			}{nil, io.EOF}, // then exit
		),
		w,
		&TunHandlerMockCrypto{},
	)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	// Writer should be called once even for zero-length payload (Encrypt returns empty slice).
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}
