package presentation

import (
	"tungo/application"
)

type ServerAppDependencies interface {
	TunManager() application.TunManager
}

type ServerDependencies struct {
	tunManager application.TunManager
}

func NewServerDependencies(tunManager application.TunManager) *ServerDependencies {
	return &ServerDependencies{
		tunManager: tunManager,
	}
}
