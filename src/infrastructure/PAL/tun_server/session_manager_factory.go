package tun_server

import "tungo/infrastructure/tunnel/session"

type sessionManagerFactory struct{}

func newSessionManagerFactory() sessionManagerFactory {
	return sessionManagerFactory{}
}

func (c *sessionManagerFactory) createManager() session.Repository {
	return session.NewDefaultRepository()
}

func (c *sessionManagerFactory) createConcurrentManager() session.Repository {
	sessionManager := c.createManager()
	return session.NewConcurrentRepository(sessionManager)
}
