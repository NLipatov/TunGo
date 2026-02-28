package server

import (
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/tunnel/session"
)

func NewRuntime(_ server.ConfigurationManager) (*Runtime, error) {
	return &Runtime{
		sessionRevoker: session.NewCompositeSessionRevoker(),
	}, nil
}
