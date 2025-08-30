package tun_server_test

import (
	"testing"
	"tungo/application"
	"tungo/infrastructure/PAL/tun_server"
)

type dummyWorker struct{}

func (dummyWorker) HandleTun() error       { return nil }
func (dummyWorker) HandleTransport() error { return nil }

func TestServerTrafficRouterFactory_CreateRouter(t *testing.T) {
	f := tun_server.NewServerTrafficRouterFactory()
	router := f.CreateRouter(dummyWorker{})
	if router == nil {
		t.Fatal("expected non-nil router")
	}
	if _, ok := router.(application.TrafficRouter); !ok {
		t.Errorf("expected application.TrafficRouter, got %T", router)
	}
}
