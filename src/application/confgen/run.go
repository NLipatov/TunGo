package confgen

import (
	"encoding/json"
	"fmt"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/stat"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/host_resolver"
)

func Run() error {
	manager, err := newServerConfigurationManager()
	if err != nil {
		return err
	}
	if err := prepareServerKeys(manager); err != nil {
		return fmt.Errorf("key preparation failed: %w", err)
	}
	generator := NewGenerator(
		manager,
		&primitives.DefaultKeyDeriver{},
		host_resolver.NewDialResolver(),
	)
	conf, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("configuration generation failed: %w", err)
	}
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("configuration generation failed: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func newServerConfigurationManager() (serverConfiguration.ConfigurationManager, error) {
	resolver := serverConfiguration.NewServerResolver()
	manager, err := serverConfiguration.NewManager(resolver, stat.NewDefaultStat())
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	return manager, nil
}

func prepareServerKeys(manager serverConfiguration.ConfigurationManager) error {
	keyManager := serverConfiguration.NewX25519KeyManager(manager)
	if err := keyManager.PrepareKeys(); err != nil {
		return fmt.Errorf("could not prepare keys: %w", err)
	}
	return nil
}
