package client_test

import (
	"errors"
	"reflect"
	"testing"
	"tungo/infrastructure/PAL/configuration/client"
	clientRunners "tungo/presentation/runners/client"
)

type mockConfigurationManager struct {
	conf *client.Configuration
	err  error
}

func (d *mockConfigurationManager) Configuration() (*client.Configuration, error) {
	return d.conf, d.err
}

func newDummyConfig() *client.Configuration {
	return &client.Configuration{}
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
