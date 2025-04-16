package presentation

import (
	"fmt"
	"tungo/application"
	"tungo/infrastructure/platform_tun"
	"tungo/infrastructure/routing_layer/client_routing/factory"
	"tungo/settings/client_configuration"
)

type ClientAppDependencies interface {
	Initialize() error
	Configuration() client_configuration.Configuration
	ConnectionFactory() application.ConnectionFactory
	WorkerFactory() application.TunWorkerFactory
	TunManager() application.TunManager
}

type ClientDependencies struct {
	conf       client_configuration.Configuration
	conn       application.ConnectionFactory
	worker     application.TunWorkerFactory
	tun        application.TunManager
	cfgManager client_configuration.ClientConfigurationManager
}

func NewClientDependencies(cfgManager client_configuration.ClientConfigurationManager) ClientAppDependencies {
	return &ClientDependencies{cfgManager: cfgManager}
}

func (c *ClientDependencies) Initialize() error {
	conf, err := c.cfgManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read client configuration: %w", err)
	}

	c.conn = factory.NewConnectionFactory(*conf)
	c.worker = factory.NewWorkerFactory(*conf)
	c.tun, err = platform_tun.NewPlatformTunManager(*conf)
	if err != nil {
		return fmt.Errorf("failed to configure tun: %w", err)
	}

	c.conf = *conf
	return nil
}

func (c *ClientDependencies) Configuration() client_configuration.Configuration {
	return c.conf
}

func (c *ClientDependencies) ConnectionFactory() application.ConnectionFactory {
	return c.conn
}

func (c *ClientDependencies) WorkerFactory() application.TunWorkerFactory {
	return c.worker
}

func (c *ClientDependencies) TunManager() application.TunManager {
	return c.tun
}
