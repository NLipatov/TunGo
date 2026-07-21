package server

import (
	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/tunnel/session"
)

func NewRuntime(_ appConfiguration.ServerRuntimeConfiguration) (*Runtime, error) {
	return &Runtime{
		sessionRevoker: session.NewCompositeSessionRevoker(),
	}, nil
}
