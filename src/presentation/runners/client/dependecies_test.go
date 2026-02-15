package client_test

import (
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
	clientRunners "tungo/presentation/runners/client"
)

type mockConfigurationManager struct {
	conf *client.Configuration
	err  error
}

func mustHost(raw string) settings.Host {
	h, err := settings.NewHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

func mustPrefix(raw string) netip.Prefix {
	return netip.MustParsePrefix(raw)
}

func mustAddr(raw string) netip.Addr {
	return netip.MustParseAddr(raw)
}

func (d *mockConfigurationManager) Configuration() (*client.Configuration, error) {
	return d.conf, d.err
}

func newDummyConfig() *client.Configuration {
	return &client.Configuration{
		UDPSettings: settings.Settings{
			InterfaceName:    "udp_dependencies_test_0",
			IPv4Subnet:  mustPrefix("10.0.1.0/24"),
			IPv4IP:      mustAddr("10.0.1.1"),
			Host:             mustHost("1.2.3.4"),
			Port:             1010,
			MTU:              1000,
			Protocol:         settings.UDP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
		},
		Protocol: settings.UDP,
	}
}

func TestClientDependencies_InitializeSuccess(t *testing.T) {
	dcm := &mockConfigurationManager{
		conf: newDummyConfig(),
		err:  nil,
	}
	deps := clientRunners.NewDependencies(dcm)
	if err := deps.Initialize(); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}

	gotConf := deps.Configuration()
	wantConf := *newDummyConfig()
	if !reflect.DeepEqual(gotConf, wantConf) {
		t.Errorf("Configuration() = %#v; want %#v", gotConf, wantConf)
	}

	if deps.ConnectionFactory() == nil {
		t.Error("ConnectionFactory() is nil")
	}
	if deps.WorkerFactory() == nil {
		t.Error("WorkerFactory() is nil")
	}
	if deps.TunManager() == nil {
		t.Error("TunManager() is nil")
	}
}

func TestClientDependencies_InitializeError(t *testing.T) {
	dcm := &mockConfigurationManager{
		conf: nil,
		err:  errors.New("dummy error"),
	}
	deps := clientRunners.NewDependencies(dcm)
	if err := deps.Initialize(); err == nil {
		t.Error("Expected error from Initialize(), got nil")
	}
}
