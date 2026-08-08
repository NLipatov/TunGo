package client

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"testing"
)

var errClientTest = errors.New("test error")

type clientTestTransport struct {
	closed int
	remote netip.AddrPort
}

func (*clientTestTransport) Read([]byte) (int, error)         { return 0, io.EOF }
func (*clientTestTransport) Write(packet []byte) (int, error) { return len(packet), nil }
func (t *clientTestTransport) Close() error {
	t.closed++
	return nil
}
func (t *clientTestTransport) RemoteAddrPort() netip.AddrPort { return t.remote }

type clientTestDevice struct {
	closed int
}

func (*clientTestDevice) Read([]byte) (int, error)         { return 0, io.EOF }
func (*clientTestDevice) Write(packet []byte) (int, error) { return len(packet), nil }
func (d *clientTestDevice) Close() error {
	d.closed++
	return nil
}

type clientTestTunManager struct {
	device    io.ReadWriteCloser
	createErr error
	endpoints []netip.AddrPort
}

func (m *clientTestTunManager) CreateDevice() (io.ReadWriteCloser, error) {
	return m.device, m.createErr
}
func (*clientTestTunManager) DisposeDevices() error { return nil }
func (m *clientTestTunManager) SetRouteEndpoint(endpoint netip.AddrPort) {
	m.endpoints = append(m.endpoints, endpoint)
}

func TestClientRunOwnsSessionLifecycle(t *testing.T) {
	remote := netip.MustParseAddrPort("192.0.2.1:443")
	transport := &clientTestTransport{remote: remote}
	device := &clientTestDevice{}
	manager := &clientTestTunManager{device: device}
	runCalled := false
	readyCalled := false
	client := &Client{
		tunManager: manager,
		establish: func(context.Context) (io.ReadWriteCloser, crypto, rekeyController, error) {
			return transport, nil, nil, nil
		},
		runTunnel: func(
			context.Context,
			io.ReadWriteCloser,
			io.ReadWriteCloser,
			crypto,
			rekeyController,
		) (func() error, error) {
			return func() error {
				runCalled = true
				return errClientTest
			}, nil
		},
	}

	if err := client.Run(context.Background(), func() { readyCalled = true }); !errors.Is(err, errClientTest) {
		t.Fatalf("Run() error = %v, want %v", err, errClientTest)
	}
	if !readyCalled || !runCalled {
		t.Fatalf("ready=%v run=%v, want both true", readyCalled, runCalled)
	}
	if transport.closed != 1 || device.closed != 1 {
		t.Fatalf("closed transport=%d device=%d, want 1 each", transport.closed, device.closed)
	}
	if len(manager.endpoints) != 2 || manager.endpoints[0].IsValid() || manager.endpoints[1] != remote {
		t.Fatalf("route endpoints = %v, want empty then %v", manager.endpoints, remote)
	}
}

func TestClientRunClosesTransportAfterTunCreationFailure(t *testing.T) {
	transport := &clientTestTransport{}
	manager := &clientTestTunManager{createErr: errClientTest}
	client := &Client{
		tunManager: manager,
		establish: func(context.Context) (io.ReadWriteCloser, crypto, rekeyController, error) {
			return transport, nil, nil, nil
		},
	}

	if err := client.Run(context.Background(), func() { t.Fatal("must not become ready") }); !errors.Is(err, errClientTest) {
		t.Fatalf("Run() error = %v, want %v", err, errClientTest)
	}
	if transport.closed != 1 {
		t.Fatalf("transport Close() calls = %d, want 1", transport.closed)
	}
}

func TestClientRunClosesResourcesAfterTunnelSetupFailure(t *testing.T) {
	transport := &clientTestTransport{}
	device := &clientTestDevice{}
	client := &Client{
		tunManager: &clientTestTunManager{device: device},
		establish: func(context.Context) (io.ReadWriteCloser, crypto, rekeyController, error) {
			return transport, nil, nil, nil
		},
		runTunnel: func(context.Context, io.ReadWriteCloser, io.ReadWriteCloser, crypto, rekeyController) (func() error, error) {
			return nil, errClientTest
		},
	}

	if err := client.Run(context.Background(), func() { t.Fatal("must not become ready") }); !errors.Is(err, errClientTest) {
		t.Fatalf("Run() error = %v, want %v", err, errClientTest)
	}
	if transport.closed != 1 || device.closed != 1 {
		t.Fatalf("closed transport=%d device=%d, want 1 each", transport.closed, device.closed)
	}
}

func TestClientRunStopsAfterConnectionFailure(t *testing.T) {
	manager := &clientTestTunManager{}
	client := &Client{
		tunManager: manager,
		establish: func(context.Context) (io.ReadWriteCloser, crypto, rekeyController, error) {
			return nil, nil, nil, errClientTest
		},
	}

	if err := client.Run(context.Background(), func() { t.Fatal("must not become ready") }); !errors.Is(err, errClientTest) {
		t.Fatalf("Run() error = %v, want %v", err, errClientTest)
	}
}
