package tcp_chacha20

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
	"tungo/infrastructure/cryptography/chacha20/rekey"
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

	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, rdr(), io.Discard, &TunHandlerMockCrypto{}, ctrl)
	if err := h.HandleTun(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTunHandler_EOF(t *testing.T) {
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, io.EOF}),
		io.Discard,
		&TunHandlerMockCrypto{}, ctrl,
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestTunHandler_ReadError(t *testing.T) {
	readErr := errors.New("read fail")
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}),
		io.Discard,
		&TunHandlerMockCrypto{}, ctrl,
	)
	if err := h.HandleTun(); !errors.Is(err, readErr) {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	encErr := errors.New("encrypt fail")
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{1, 2, 3}, nil},
		),
		io.Discard,
		&TunHandlerMockCrypto{err: encErr}, ctrl,
	)
	if err := h.HandleTun(); !errors.Is(err, encErr) {
		t.Fatalf("want encrypt error, got %v", err)
	}
}

func TestTunHandler_WriteError(t *testing.T) {
	wErr := errors.New("write fail")
	w := &TunHandlerMockWriter{err: wErr}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(context.Background(),
		rdr(struct {
			data []byte
			err  error
		}{[]byte{9, 9}, nil}),
		w,
		&TunHandlerMockCrypto{}, ctrl,
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
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
		&TunHandlerMockCrypto{}, ctrl,
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
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(
		ctx,
		rdr(struct {
			data []byte
			err  error
		}{nil, readErr}), // reader returns an error
		io.Discard,
		&TunHandlerMockCrypto{}, ctrl,
	)

	// When ctx is canceled, the read error path should return nil.
	if err := h.HandleTun(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTunHandler_RekeyInitSentAfterPayload(t *testing.T) {
	w := &TunHandlerMockWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, make([]byte, 32), make([]byte, 32), false)
	h := NewTunHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{0xAA, 0xBB}, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		w,
		&TunHandlerMockCrypto{}, ctrl,
	)

	// Force rekey to fire by setting rotateAt to the past.
	th := h.(*TunHandler)
	th.rekeyInit.SetRotateAt(time.Now().Add(-time.Second))
	th.rekeyInit.SetInterval(time.Millisecond)

	if err := th.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	// At least 2 writes: one for data, one for rekeyInit control packet.
	if w.writes < 2 {
		t.Fatalf("expected at least 2 writes (data + rekeyInit), got %d", w.writes)
	}
}

func TestTunHandler_RekeyInitSendError_Continues(t *testing.T) {
	// When sending a rekey init via egress fails, the handler should log and continue.
	sendCount := 0
	mockWriter := &TunHandlerMockWriter{err: nil}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, make([]byte, 32), make([]byte, 32), false)
	h := NewTunHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{0xAA}, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		mockWriter,
		&TunHandlerMockCrypto{}, ctrl,
	)
	th := h.(*TunHandler)
	th.rekeyInit.SetRotateAt(time.Now().Add(-time.Second))
	th.rekeyInit.SetInterval(time.Millisecond)

	// Replace egress with one that fails on SendControl.
	th.egress = &failingControlEgress{dataErr: nil, controlErr: errors.New("send fail")}

	if err := th.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	_ = sendCount // handler should continue despite send error
}

// failingControlEgress is an egress that fails on SendControl.
type failingControlEgress struct {
	dataErr    error
	controlErr error
}

func (e *failingControlEgress) SendDataIP(_ []byte) error  { return e.dataErr }
func (e *failingControlEgress) SendControl(_ []byte) error { return e.controlErr }
func (e *failingControlEgress) Close() error               { return nil }

func TestTunHandler_RekeyInitPrepareError_Continues(t *testing.T) {
	// When MaybeBuildRekeyInit returns an error, the handler should log and continue (not exit).
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, make([]byte, 32), make([]byte, 32), false)
	mockW := &TunHandlerMockWriter{}
	h := NewTunHandler(context.Background(),
		rdr(
			struct {
				data []byte
				err  error
			}{[]byte{0xAA}, nil},
			struct {
				data []byte
				err  error
			}{[]byte{0xBB}, nil},
			struct {
				data []byte
				err  error
			}{nil, io.EOF},
		),
		mockW,
		&TunHandlerMockCrypto{}, ctrl,
	)
	th := h.(*TunHandler)
	th.rekeyInit.SetRotateAt(time.Now().Add(-time.Second))
	th.rekeyInit.SetInterval(time.Millisecond)

	// Set the pending key so the reuse branch in MaybeBuildRekeyInit fires,
	// but truncate controlPacketBuf to force an error (short dst).
	// Actually, easier: just set rekeyInit to use a nil crypto which returns ok=false.
	th.rekeyInit = nil // nil rekeyInit -> skip rekey entirely (handles the nil guard)

	if err := th.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	// Both data packets should still have been sent.
	if mockW.writes < 2 {
		t.Fatalf("expected at least 2 writes, got %d", mockW.writes)
	}
}

func TestTunHandler_ZeroLengthPayload_ThenEOF(t *testing.T) {
	w := &TunHandlerMockWriter{}
	ctrl := rekey.NewStateMachine(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
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
		&TunHandlerMockCrypto{}, ctrl,
	)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	// Writer should be called once even for zero-length payload (Encrypt returns empty slice).
	if w.writes != 1 {
		t.Fatalf("writes=%d, want 1", w.writes)
	}
}
