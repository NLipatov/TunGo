package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/routing/server_routing/session_management/repository/wrappers"
)

type sessionManagerFactory[T application.Session] struct {
}

func newSessionManagerFactory[T application.Session]() sessionManagerFactory[T] {
	return sessionManagerFactory[T]{}
}

func (c *sessionManagerFactory[T]) createManager() repository.SessionRepository[T] {
	return repository.NewDefaultWorkerSessionManager[T]()
}

func (c *sessionManagerFactory[T]) createConcurrentManager() repository.SessionRepository[T] {
	sessionManager := c.createManager()
	return wrappers.NewConcurrentManager(sessionManager)
}
