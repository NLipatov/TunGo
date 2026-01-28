package client_factory

import (
	"context"
	"errors"
	"io"
	"testing"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// -------------------- Mocks (prefixed with "RouterFactory") --------------------

// RouterFactoryConnectionFactoryMock mocks connection.Factory.
type RouterFactoryConnectionFactoryMock struct {
	Conn   connection.Transport
	Crypto connection.Crypto
	Err    error

	Called bool
}

func (m *RouterFactoryConnectionFactoryMock) EstablishConnection(_ context.Context) (connection.Transport, connection.Crypto, *rekey.Controller, error) {
	m.Called = true
	return m.Conn, m.Crypto, nil, m.Err
}

// RouterFactoryTunClientManagerMock mocks tun.ClientManager.
type RouterFactoryTunClientManagerMock struct {
	Device tun.Device
	Err    error

	Called bool
}

func (m *RouterFactoryTunClientManagerMock) CreateDevice() (tun.Device, error) {
	m.Called = true
	return m.Device, m.Err
}

func (m *RouterFactoryTunClientManagerMock) DisposeDevices() error {
	// Not needed for these tests.
	return nil
}

// RouterFactoryClientWorkerFactoryMock mocks connection.ClientWorkerFactory.
type RouterFactoryClientWorkerFactoryMock struct {
	Worker routing.Worker
	Err    error

	// captured args for assertions
	Ctx    context.Context
	Conn   connection.Transport
	Tun    io.ReadWriteCloser
	Crypto connection.Crypto
	Ctrl   *rekey.Controller

	Called bool
}

func (m *RouterFactoryClientWorkerFactoryMock) CreateWorker(
	ctx context.Context,
	conn connection.Transport,
	tun io.ReadWriteCloser,
	cryptographyService connection.Crypto,
	controller *rekey.Controller,
) (routing.Worker, error) {
	m.Called = true
	m.Ctx = ctx
	m.Conn = conn
	m.Tun = tun
	m.Crypto = cryptographyService
	m.Ctrl = controller
	return m.Worker, m.Err
}

// RouterFactoryTransportMock is a simple transport mock implementing connection.Transport.
type RouterFactoryTransportMock struct{}

func (r *RouterFactoryTransportMock) Write(b []byte) (int, error) { return len(b), nil }
func (r *RouterFactoryTransportMock) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (r *RouterFactoryTransportMock) Close() error                { return nil }

// RouterFactoryDeviceMock is a simple device mock implementing tun.Device.
type RouterFactoryDeviceMock struct{}

func (d *RouterFactoryDeviceMock) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (d *RouterFactoryDeviceMock) Write(b []byte) (int, error) { return len(b), nil }
func (d *RouterFactoryDeviceMock) Close() error                { return nil }

// RouterFactoryWorkerMock implements application.Worker (HandleTun, HandleTransport).
type RouterFactoryWorkerMock struct {
	HandledTun       bool
	HandledTransport bool
}

func (w *RouterFactoryWorkerMock) HandleTun() error {
	w.HandledTun = true
	return nil
}

func (w *RouterFactoryWorkerMock) HandleTransport() error {
	w.HandledTransport = true
	return nil
}

// RouterFactoryCryptoMock implements connection.Crypto.
type RouterFactoryCryptoMock struct{}

func (c *RouterFactoryCryptoMock) Encrypt(plaintext []byte) ([]byte, error) {
	// trivial identity "encryption" for tests
	out := make([]byte, len(plaintext))
	copy(out, plaintext)
	return out, nil
}

func (c *RouterFactoryCryptoMock) Decrypt(ciphertext []byte) ([]byte, error) {
	out := make([]byte, len(ciphertext))
	copy(out, ciphertext)
	return out, nil
}

// -------------------- Tests --------------------

func TestRouterFactory_CreateRouter_EstablishConnectionError(t *testing.T) {
	factory := NewRouterFactory()

	connFactoryMock := &RouterFactoryConnectionFactoryMock{Err: errors.New("connect fail")}
	tunManager := &RouterFactoryTunClientManagerMock{}
	workerFactory := &RouterFactoryClientWorkerFactoryMock{}

	router, conn, device, err := factory.CreateRouter(context.Background(), connFactoryMock, tunManager, workerFactory)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if router != nil || conn != nil || device != nil {
		t.Fatalf("expected nils on error, got router=%v conn=%v device=%v", router, conn, device)
	}
	if !connFactoryMock.Called {
		t.Fatalf("expected EstablishConnection to be called")
	}
}

func TestRouterFactory_CreateRouter_CreateDeviceError(t *testing.T) {
	factory := NewRouterFactory()

	connMock := &RouterFactoryTransportMock{}
	connFactoryMock := &RouterFactoryConnectionFactoryMock{Conn: connMock, Crypto: nil, Err: nil}
	tunManager := &RouterFactoryTunClientManagerMock{Err: errors.New("tun fail")}
	workerFactory := &RouterFactoryClientWorkerFactoryMock{}

	router, conn, device, err := factory.CreateRouter(context.Background(), connFactoryMock, tunManager, workerFactory)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if router != nil || conn != nil || device != nil {
		t.Fatalf("expected nils on device-create error, got router=%v conn=%v device=%v", router, conn, device)
	}
	if !tunManager.Called {
		t.Fatalf("expected CreateDevice to be called")
	}
}

func TestRouterFactory_CreateRouter_CreateWorkerError(t *testing.T) {
	factory := NewRouterFactory()

	connMock := &RouterFactoryTransportMock{}
	deviceMock := &RouterFactoryDeviceMock{}
	connFactoryMock := &RouterFactoryConnectionFactoryMock{Conn: connMock, Crypto: nil, Err: nil}
	tunManager := &RouterFactoryTunClientManagerMock{Device: deviceMock}
	workerFactory := &RouterFactoryClientWorkerFactoryMock{Err: errors.New("worker fail")}

	router, conn, device, err := factory.CreateRouter(context.Background(), connFactoryMock, tunManager, workerFactory)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if router != nil || conn != nil || device != nil {
		t.Fatalf("expected nils on worker-create error, got router=%v conn=%v device=%v", router, conn, device)
	}
	if !workerFactory.Called {
		t.Fatalf("expected CreateWorker to be called")
	}
}

func TestRouterFactory_CreateRouter_Success(t *testing.T) {
	factory := NewRouterFactory()

	connMock := &RouterFactoryTransportMock{}
	deviceMock := &RouterFactoryDeviceMock{}
	workerMock := &RouterFactoryWorkerMock{}
	cryptoMock := &RouterFactoryCryptoMock{}

	connFactoryMock := &RouterFactoryConnectionFactoryMock{Conn: connMock, Crypto: cryptoMock, Err: nil}
	tunManager := &RouterFactoryTunClientManagerMock{Device: deviceMock}
	workerFactory := &RouterFactoryClientWorkerFactoryMock{Worker: workerMock}

	ctx := context.Background()
	router, conn, device, err := factory.CreateRouter(ctx, connFactoryMock, tunManager, workerFactory)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if router == nil {
		t.Fatalf("expected non-nil router")
	}
	if conn != connMock {
		t.Fatalf("expected returned conn to be the same mock")
	}
	if device != deviceMock {
		t.Fatalf("expected returned device to be the same mock")
	}
	if !workerFactory.Called {
		t.Fatalf("expected CreateWorker to be called")
	}
	// Check captured arguments in worker factory
	if workerFactory.Conn != connMock {
		t.Fatalf("worker factory received wrong conn")
	}
	if workerFactory.Tun != deviceMock {
		t.Fatalf("worker factory received wrong device (tun)")
	}
	if workerFactory.Crypto != cryptoMock {
		t.Fatalf("worker factory received wrong crypto")
	}
	if workerFactory.Ctx != ctx {
		t.Fatalf("worker factory received wrong context")
	}
}
