package tun_server

import (
	"context"
	"io"
	"tungo/application/network/routing"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
)

type ServerWorkerFactory struct {
	configurationManager server.ConfigurationManager
	sessionRevoker       *session.CompositeSessionRevoker
}

func NewServerWorkerFactory(manager server.ConfigurationManager) *ServerWorkerFactory {
	return &ServerWorkerFactory{
		configurationManager: manager,
		sessionRevoker:       session.NewCompositeSessionRevoker(),
	}
}

// SessionRevoker returns the composite session revoker.
// Used by ConfigWatcher to revoke sessions when AllowedPeers changes.
func (s *ServerWorkerFactory) SessionRevoker() *session.CompositeSessionRevoker {
	return s.sessionRevoker
}

func (s *ServerWorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
	_ settings.Settings,
) (routing.Worker, error) {
	panic("not implemented")
}
