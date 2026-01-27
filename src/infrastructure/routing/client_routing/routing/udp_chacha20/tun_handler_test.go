package udp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
	"tungo/application/network/rekey"
	"tungo/domain/network/service"

	"golang.org/x/crypto/chacha20poly1305"
)

// fakeCrypto implements application.Crypto for testing
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

type tunHandlerTestCancelOnEncrypt struct {
	cancel context.CancelFunc
	err    error
	prefix []byte
}

func (c *tunHandlerTestCancelOnEncrypt) Encrypt(p []byte) ([]byte, error) {
	if c.cancel != nil {
		c.cancel()
	}
	if c.err != nil {
		return nil, c.err
	}
	return append(c.prefix, p...), nil
}
func (c *tunHandlerTestCancelOnEncrypt) Decrypt(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("unused")
}

type tunHandlerTestCancelWriter struct {
	cancel context.CancelFunc
}

func (w *tunHandlerTestCancelWriter) Write(_ []byte) (int, error) {
	if w.cancel != nil {
		w.cancel()
	}
	return 0, errors.New("write fail")
}

func TestHandleTun_ImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, &fakeReader{readFunc: func(p []byte) (int, error) {
		// should not be called
		t.Fatal("Read called despite context cancelled")
		return 0, nil
	}}, &fakeWriter{}, &tunhandlerTestRakeCrypto{}, ctrl, service.NewDefaultPacketHandler())

	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil on immediate cancel, got %v", err)
	}
}

func TestHandleTun_ReadError(t *testing.T) {
	errRead := errors.New("read failure")
	ctx := context.Background()
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, &fakeReader{readFunc: func(p []byte) (int, error) {
		return 0, errRead
	}}, &fakeWriter{}, &tunhandlerTestRakeCrypto{}, ctrl, service.NewDefaultPacketHandler())

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
	h := NewTunHandler(ctx, reader, &fakeWriter{}, &tunhandlerTestRakeCrypto{err: errEnc}, rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false), service.NewDefaultPacketHandler())

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
	h := NewTunHandler(ctx, reader, writer, &tunhandlerTestRakeCrypto{prefix: []byte("x:")}, rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false), service.NewDefaultPacketHandler())

	if err := h.HandleTun(); err == nil || err.Error() != fmt.Sprintf("could not write packet to transport: %v", errWrite) {
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

	h := NewTunHandler(ctx, reader, writer, crypto, rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false), service.NewDefaultPacketHandler())

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
	zeros := make([]byte, chacha20poly1305.NonceSize)
	want := append([]byte("pre-"), append(zeros, dummyData...)...)
	if len(writer.data) != 1 || !bytes.Equal(writer.data[0], want) {
		t.Errorf("expected written %v, got %v", want, writer.data)
	}
}
func TestHandleTun_ReadErrorAfterCancel_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// reader cancels context inside Read, then returns an error
	r := &fakeReader{readFunc: func(p []byte) (int, error) {
		cancel()
		return 0, errors.New("read fail")
	}}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, r, &fakeWriter{}, &tunhandlerTestRakeCrypto{}, ctrl, service.NewDefaultPacketHandler())

	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil because ctx canceled, got %v", err)
	}
}

func TestHandleTun_EncryptErrorAfterCancel_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// first read yields data; Encrypt will cancel ctx and then return error
	r := &fakeReader{readFunc: func(p []byte) (int, error) {
		copy(p, []byte{1, 2, 3})
		return 3, nil
	}}
	crypt := &tunHandlerTestCancelOnEncrypt{cancel: cancel, err: errors.New("enc fail")}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, r, &fakeWriter{}, crypt, ctrl, service.NewDefaultPacketHandler())

	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil because ctx canceled before encrypt error handling, got %v", err)
	}
}

func TestHandleTun_WriteErrorAfterCancel_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &fakeReader{readFunc: func(p []byte) (int, error) {
		copy(p, []byte{4, 5, 6})
		return 3, nil
	}}
	w := &tunHandlerTestCancelWriter{cancel: cancel}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, r, w, &tunhandlerTestRakeCrypto{prefix: []byte("x:")}, ctrl, service.NewDefaultPacketHandler())

	if err := h.HandleTun(); err != nil {
		t.Fatalf("expected nil because ctx canceled before write error handling, got %v", err)
	}
}

func TestHandleTun_ReadReturnsNAndEOF_OneWriteThenEOF(t *testing.T) {
	// First call: n>0 and io.EOF. Handler must:
	//  - process n>0 (encrypt+write)
	//  - then see err!=nil and return wrapped read error.
	ctx := context.Background()

	first := true
	r := &fakeReader{readFunc: func(p []byte) (int, error) {
		if first {
			first = false
			copy(p, []byte{7, 8, 9})
			return 3, io.EOF
		}
		return 0, io.EOF
	}}
	w := &fakeWriter{}
	ctrl := rekey.NewController(dummyRekeyer{}, []byte("c2s"), []byte("s2c"), false)
	h := NewTunHandler(ctx, r, w, &tunhandlerTestRakeCrypto{prefix: []byte("pre-")}, ctrl, service.NewDefaultPacketHandler())

	err := h.HandleTun()
	if err == nil || err.Error() != "could not read a packet from TUN: EOF" {
		t.Fatalf("expected wrapped EOF, got %v", err)
	}
	if len(w.data) != 1 {
		t.Fatalf("expected exactly one write, got %d", len(w.data))
	}
	// payload layout: prefix || 12B nonce || [7,8,9]
	zeros := make([]byte, chacha20poly1305.NonceSize)
	want := append([]byte("pre-"), append(zeros, []byte{7, 8, 9}...)...)
	if !bytes.Equal(w.data[0], want) {
		t.Fatalf("written mismatch: got %v, want %v", w.data[0], want)
	}
}
