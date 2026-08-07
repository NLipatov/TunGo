package server_test

import (
	"testing"

	"tungo/application/network/routing"
	tunnelServer "tungo/infrastructure/PAL/tunnel/server"
)

func TestTrafficRouterFactory_CreateRouter(t *testing.T) {
	f := tunnelServer.NewTrafficRouterFactory()
	router := f.CreateRouter(routing.Endpoints{
		RunTun: func() error { return nil }, RunTransport: func() error { return nil },
	})
	if router == nil {
		t.Fatal("expected non-nil router")
	}
}
