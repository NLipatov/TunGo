package client

import (
	"fmt"
	"tungo/application"
	"tungo/infrastructure/PAL/tun_manager"
	"tungo/infrastructure/routing/client_routing/client_factory"
	"tungo/settings/client_configuration"
)

type AppDependencies interface {
	Initialize() error
	Configuration() client_configuration.Configuration
	ConnectionFactory() application.ConnectionFactory
	WorkerFactory() application.ClientWorkerFactory
	TunManager() application.ClientTunManager
}

type Dependencies struct {
	conf       client_configuration.Configuration
	conn       application.ConnectionFactory
	worker     application.ClientWorkerFactory
	tun        application.ClientTunManager
	cfgManager client_configuration.ClientConfigurationManager
}

func NewDependencies(cfgManager client_configuration.ClientConfigurationManager) AppDependencies {
	return &Dependencies{cfgManager: cfgManager}
}

func (c *Dependencies) Initialize() error {
	conf, err := c.cfgManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read client configuration: %w", err)
	}

	c.conn = client_factory.NewConnectionFactory(*conf)
	c.worker = client_factory.NewWorkerFactory(*conf)
	c.tun, err = tun_manager.NewPlatformTunManager(*conf)
	if err != nil {
		return fmt.Errorf("failed to configure tun: %w", err)
	}

	c.conf = *conf
	return nil
}

func (c *Dependencies) Configuration() client_configuration.Configuration {
	return c.conf
}

func (c *Dependencies) ConnectionFactory() application.ConnectionFactory {
	return c.conn
}

func (c *Dependencies) WorkerFactory() application.ClientWorkerFactory {
	return c.worker
}

func (c *Dependencies) TunManager() application.ClientTunManager {
	return c.tun
}
