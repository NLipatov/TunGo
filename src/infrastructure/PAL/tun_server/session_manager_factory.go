package tun_server

import "tungo/infrastructure/routing/server_routing/session_management"

type sessionManagerFactory[T session_management.ClientSession] struct {
}

func newSessionManagerFactory[T session_management.ClientSession]() sessionManagerFactory[T] {
	return sessionManagerFactory[T]{}
}

func (c *sessionManagerFactory[T]) createManager() session_management.WorkerSessionManager[T] {
	return session_management.NewDefaultWorkerSessionManager[T]()
}

func (c *sessionManagerFactory[T]) createConcurrentManager() session_management.WorkerSessionManager[T] {
	sessionManager := c.createManager()
	return session_management.NewConcurrentManager(sessionManager)
}
