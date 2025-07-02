package server_configuration

type SessionLifetimeManager interface {
	PrepareSessionLifetime() error
}

type DefaultSessionLifetimeManager struct {
	config  *Configuration
	manager ServerConfigurationManager
}

func NewDefaultSessionLifetimeManager(
	config *Configuration,
	manager ServerConfigurationManager,
) SessionLifetimeManager {
	return &DefaultSessionLifetimeManager{
		config:  config,
		manager: manager,
	}
}

func (d *DefaultSessionLifetimeManager) PrepareSessionLifetime() error {
	if d.hasValidSessionLifetime() {
		return nil
	}

	return d.manager.InjectSessionTtlIntervals(DefaultSessionTtl, DefaultSessionCleanupInterval)
}

func (d *DefaultSessionLifetimeManager) hasValidSessionLifetime() bool {
	if d.config.TCPSettings.SessionLifetime.Ttl > 0 && d.config.TCPSettings.SessionLifetime.CleanupInterval > 0 &&
		d.config.UDPSettings.SessionLifetime.Ttl > 0 && d.config.UDPSettings.SessionLifetime.CleanupInterval > 0 {
		return true
	}

	return false
}
