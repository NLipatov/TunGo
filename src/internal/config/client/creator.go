package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Creator struct {
	resolver Resolver
}

func NewCreator(resolver Resolver) *Creator {
	return &Creator{
		resolver: resolver,
	}
}

func (d *Creator) Create(configuration Configuration, name string) error {
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.ContainsAny(name, "\x00") {
		return fmt.Errorf("invalid configuration name %q: must not contain path separators", name)
	}

	serialized, err := json.MarshalIndent(configuration, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal client configuration: %w", err)
	}

	defaultConfPath, err := d.resolver.Resolve()
	if err != nil {
		return fmt.Errorf("failed to resolve default client configuration path: %w", err)
	}

	confPath := fmt.Sprintf("%s.%s", defaultConfPath, name)
	if err := os.MkdirAll(filepath.Dir(confPath), 0700); err != nil {
		return fmt.Errorf("failed to create client configuration directory: %w", err)
	}

	if err := os.WriteFile(confPath, serialized, 0600); err != nil {
		return fmt.Errorf("failed to write client configuration %q: %w", confPath, err)
	}

	return nil
}
