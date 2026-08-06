package server_test

import (
	"testing"
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
}
