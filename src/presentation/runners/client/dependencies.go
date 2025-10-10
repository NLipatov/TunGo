package client

import (
	"fmt"
	"tungo/application"
	"tungo/application/network/tun"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/tun_client"
	"tungo/infrastructure/routing/client_routing/client_factory"
)

type AppDependencies interface {
	Initialize() error
	Configuration() clientConfiguration.Configuration
	ConnectionFactory() application.ConnectionFactory
	WorkerFactory() application.ClientWorkerFactory
	TunManager() tun.ClientManager
}

type Dependencies struct {
	conf       clientConfiguration.Configuration
	conn       application.ConnectionFactory
	worker     application.ClientWorkerFactory
	tun        tun.ClientManager
	cfgManager clientConfiguration.ClientConfigurationManager
}

func NewDependencies(cfgManager clientConfiguration.ClientConfigurationManager) AppDependencies {
	return &Dependencies{cfgManager: cfgManager}
}

func (c *Dependencies) Initialize() error {
	conf, err := c.cfgManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read client configuration: %w", err)
	}

	c.conn = client_factory.NewConnectionFactory(*conf)
	c.worker = client_factory.NewWorkerFactory(*conf)
	c.tun, err = tun_client.NewPlatformTunManager(*conf)
	if err != nil {
		return fmt.Errorf("failed to configure tun: %w", err)
	}

	c.conf = *conf
	return nil
}

func (c *Dependencies) Configuration() clientConfiguration.Configuration {
	return c.conf
}

func (c *Dependencies) ConnectionFactory() application.ConnectionFactory {
	return c.conn
}

func (c *Dependencies) WorkerFactory() application.ClientWorkerFactory {
	return c.worker
}

func (c *Dependencies) TunManager() tun.ClientManager {
	return c.tun
}
