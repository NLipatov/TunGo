package udp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// fakeCrypto implements application.CryptographyService for testing
type tunhandlerTestRakeCrypto struct {
	prefix []byte
	err    error
}

func (f *tunhandlerTestRakeCrypto) Encrypt(p []byte) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append(f.prefix, p...), nil
}

func (f *tunhandlerTestRakeCrypto) Decrypt(_ []byte) ([]byte, error) {
	// Not used in TunHandler tests
	return nil, fmt.Errorf("not implemented")
}

// fakeReader allows custom Read behavior
type fakeReader struct {
	readFunc func(p []byte) (int, error)
}

func (r *fakeReader) Read(p []byte) (int, error) {
	return r.readFunc(p)
}

// fakeWriter collects written data and can simulate errors
type fakeWriter struct {
	data [][]byte
	err  error
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	w.data = append(w.data, buf)
	return len(p), nil
}

func TestHandleTun_ImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	h := NewTunHandler(ctx, &fakeReader{readFunc: func(p []byte) (int, error) {
		// should not be called
		t.Fatal("Read called despite context cancelled")
		return 0, nil
	}}, &fakeWriter{}, &tunhandlerTestRakeCrypto{})

	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil on immediate cancel, got %v", err)
	}
}

func TestHandleTun_ReadError(t *testing.T) {
	errRead := errors.New("read failure")
	ctx := context.Background()
	h := NewTunHandler(ctx, &fakeReader{readFunc: func(p []byte) (int, error) {
		return 0, errRead
	}}, &fakeWriter{}, &tunhandlerTestRakeCrypto{})

	if err := h.HandleTun(); err == nil || err.Error() != fmt.Sprintf("could not read a packet from TUN: %v", errRead) {
		t.Fatalf("expected read error wrapped, got %v", err)
	}
}

func TestHandleTun_EncryptError(t *testing.T) {
	ctx := context.Background()
	dummyData := []byte{1, 2, 3}
	reader := &fakeReader{readFunc: func(p []byte) (int, error) {
		copy(p, dummyData)
		return len(dummyData), nil
	}}
	errEnc := errors.New("encrypt fail")
	h := NewTunHandler(ctx, reader, &fakeWriter{}, &tunhandlerTestRakeCrypto{err: errEnc})

	if err := h.HandleTun(); err == nil || err.Error() != fmt.Sprintf("could not encrypt packet: %v", errEnc) {
		t.Fatalf("expected encrypt error wrapped, got %v", err)
	}
}

func TestHandleTun_WriteError(t *testing.T) {
	ctx := context.Background()
	dummyData := []byte{4, 5, 6}
	reader := &fakeReader{readFunc: func(p []byte) (int, error) {
		copy(p, dummyData)
		return len(dummyData), nil
	}}
	errWrite := errors.New("write fail")
	writer := &fakeWriter{err: errWrite}
	h := NewTunHandler(ctx, reader, writer, &tunhandlerTestRakeCrypto{prefix: []byte("x:")})

	if err := h.HandleTun(); err == nil || err.Error() != fmt.Sprintf("could not write packet to adapter: %v", errWrite) {
		t.Fatalf("expected write error wrapped, got %v", err)
	}
}

func TestHandleTun_SuccessThenCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dummyData := []byte{7, 8, 9}
	readCount := 0
	reader := &fakeReader{readFunc: func(p []byte) (int, error) {
		readCount++
		if readCount == 1 {
			copy(p, dummyData)
			return len(dummyData), nil
		}
		// after first, block until context cancelled
		<-ctx.Done()
		// simulate read error after cancel
		return 0, errors.New("blocked read")
	}}
	writer := &fakeWriter{}
	crypto := &tunhandlerTestRakeCrypto{prefix: []byte("pre-")}

	h := NewTunHandler(ctx, reader, writer, crypto)

	done := make(chan error)
	go func() {
		done <- h.HandleTun()
	}()

	// wait for first iteration
	time.Sleep(10 * time.Millisecond)
	// cancel to exit loop
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("expected nil after cancel, got %v", err)
	}

	// verify written
	want := append([]byte("pre-"), dummyData...)
	if len(writer.data) != 1 || !bytes.Equal(writer.data[0], want) {
		t.Errorf("expected written %v, got %v", want, writer.data)
	}
}
