package client_test

import (
	"errors"
	"reflect"
	"testing"
	"tungo/presentation/runners/client"

	"tungo/settings/client_configuration"
)

type mockConfigurationManager struct {
	conf *client_configuration.Configuration
	err  error
}

func (d *mockConfigurationManager) Configuration() (*client_configuration.Configuration, error) {
	return d.conf, d.err
}

func newDummyConfig() *client_configuration.Configuration {
	return &client_configuration.Configuration{}
}

func TestClientDependencies_InitializeSuccess(t *testing.T) {
	dcm := &mockConfigurationManager{
		conf: newDummyConfig(),
		err:  nil,
	}
	deps := client.NewDependencies(dcm)
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
	deps := client.NewDependencies(dcm)
	if err := deps.Initialize(); err == nil {
		t.Error("Expected error from Initialize(), got nil")
	}
}
