package client_factory

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

// -------------------- Small test helpers / mocks (prefix WorkerFactory) --------------------

// WorkerFactoryTunMock implements io.ReadWriteCloser for unit tests.
type WorkerFactoryTunMock struct{}

func (t *WorkerFactoryTunMock) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (t *WorkerFactoryTunMock) Write(b []byte) (int, error) { return len(b), nil }
func (t *WorkerFactoryTunMock) Close() error                { return nil }

// WorkerFactoryTransportMock implements connection.Transport for non-UDP tests.
type WorkerFactoryTransportMock struct{}

func (r *WorkerFactoryTransportMock) Write(b []byte) (int, error) { return len(b), nil }
func (r *WorkerFactoryTransportMock) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (r *WorkerFactoryTransportMock) Close() error                { return nil }

// WorkerFactoryCryptoMock implements connection.Crypto (simple identity).
type WorkerFactoryCryptoMock struct{}

func (c *WorkerFactoryCryptoMock) Encrypt(plaintext []byte) ([]byte, error) {
	out := make([]byte, len(plaintext))
	copy(out, plaintext)
	return out, nil
}
func (c *WorkerFactoryCryptoMock) Decrypt(ciphertext []byte) ([]byte, error) {
	out := make([]byte, len(ciphertext))
	copy(out, ciphertext)
	return out, nil
}

// -------------------- Tests --------------------

func TestWorkerFactory_CreateWorker_UnsupportedProtocol(t *testing.T) {
	cfg := client.Configuration{
		Protocol: settings.Protocol(0xFFFF), // unknown protocol
	}

	wf := NewWorkerFactory(cfg)

	// Use a dummy transport and tun; should not be used because error occurs early.
	transport := &WorkerFactoryTransportMock{}
	tun := &WorkerFactoryTunMock{}
	crypto := &WorkerFactoryCryptoMock{}

	ctx := context.Background()
	worker, err := wf.CreateWorker(ctx, transport, tun, crypto)
	if err == nil {
		t.Fatalf("expected error for unsupported protocol, got nil and worker=%v", worker)
	}
	if worker != nil {
		t.Fatalf("expected nil worker on unsupported protocol, got %v", worker)
	}
}

func TestWorkerFactory_CreateWorker_TCP(t *testing.T) {
	// TCP path should produce a non-nil worker (uses tcp_chacha20 constructors internally).
	cfg := client.Configuration{
		Protocol: settings.TCP,
	}

	wf := NewWorkerFactory(cfg)

	transport := &WorkerFactoryTransportMock{}
	tun := &WorkerFactoryTunMock{}
	crypto := &WorkerFactoryCryptoMock{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	worker, err := wf.CreateWorker(ctx, transport, tun, crypto)
	if err != nil {
		t.Fatalf("expected no error for TCP protocol, got: %v", err)
	}
	if worker == nil {
		t.Fatalf("expected non-nil worker for TCP")
	}
}

func TestWorkerFactory_CreateWorker_WS(t *testing.T) {
	// WS path reuses TCP logic in implementation — expect non-nil worker.
	cfg := client.Configuration{
		Protocol: settings.WS,
	}

	wf := NewWorkerFactory(cfg)

	transport := &WorkerFactoryTransportMock{}
	tun := &WorkerFactoryTunMock{}
	crypto := &WorkerFactoryCryptoMock{}

	ctx := context.Background()
	worker, err := wf.CreateWorker(ctx, transport, tun, crypto)
	if err != nil {
		t.Fatalf("expected no error for WS protocol, got: %v", err)
	}
	if worker == nil {
		t.Fatalf("expected non-nil worker for WS")
	}
}

func TestWorkerFactory_CreateWorker_UDP(t *testing.T) {
	// UDP path expects conn to be *net.UDPConn (code does conn.(*net.UDPConn)).
	// Create a real UDPConn bound to localhost for the duration of test.
	laddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	udpConn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		t.Fatalf("failed to create UDP listener for test: %v", err)
	}
	defer func(udpConn *net.UDPConn) {
		_ = udpConn.Close()
	}(udpConn)

	cfg := client.Configuration{
		Protocol: settings.UDP,
	}

	wf := NewWorkerFactory(cfg)

	// note: *net.UDPConn implements io.ReadWriteCloser and also the Write/Read/Close of connection.Transport
	tun := &WorkerFactoryTunMock{}
	crypto := &WorkerFactoryCryptoMock{}

	ctx := context.Background()
	worker, err := wf.CreateWorker(ctx, udpConn, tun, crypto)
	if err != nil {
		t.Fatalf("expected no error for UDP protocol, got: %v", err)
	}
	if worker == nil {
		t.Fatalf("expected non-nil worker for UDP")
	}
}
