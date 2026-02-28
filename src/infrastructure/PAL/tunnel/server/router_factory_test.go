package server_test

import (
	"testing"
	"tungo/application/network/routing"
	tunnelServer "tungo/infrastructure/PAL/tunnel/server"
)

type dummyWorker struct{}

func (dummyWorker) HandleTun() error       { return nil }
func (dummyWorker) HandleTransport() error { return nil }

func TestTrafficRouterFactory_CreateRouter(t *testing.T) {
	f := tunnelServer.NewTrafficRouterFactory()
	router := f.CreateRouter(dummyWorker{})
	if router == nil {
		t.Fatal("expected non-nil router")
	}
	if _, ok := router.(routing.Router); !ok {
		t.Errorf("expected application.Router, got %T", router)
	}
}
