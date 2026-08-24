package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Configurations owns the active client configuration and its named alternatives.
type Configurations struct {
	activePath string
}

// Files returns the client configurations stored at the platform-specific system path.
func Files() *Configurations {
	return newConfigurations(defaultPath())
}

func newConfigurations(activePath string) *Configurations {
	return &Configurations{activePath: activePath}
}

// Active reads, defaults, and validates the active client configuration.
func (c *Configurations) Active() (*Configuration, error) {
	data, err := os.ReadFile(c.activePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("client configuration %q does not exist: %w", c.activePath, err)
		}
		return nil, fmt.Errorf("failed to read client configuration %q: %w", c.activePath, err)
	}

	configuration, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("invalid client configuration %q: %w", c.activePath, err)
	}
	return &configuration, nil
}

// List returns the names of saved alternatives. The active configuration is excluded.
func (c *Configurations) List() ([]string, error) {
	entries, err := os.ReadDir(filepath.Dir(c.activePath))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	prefix := filepath.Base(c.activePath) + "."
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		name := strings.TrimPrefix(entry.Name(), prefix)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// Import validates and stores a named client configuration.
func (c *Configurations) Import(name, rawJSON string) error {
	path, err := c.alternativePath(name)
	if err != nil {
		return err
	}

	configuration, err := decode([]byte(strings.TrimFunc(rawJSON, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.In(r, unicode.Cf)
	})))
	if err != nil {
		return fmt.Errorf("invalid client configuration: %w", err)
	}
	data, err := json.MarshalIndent(configuration, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal client configuration: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create client configuration directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write client configuration %q: %w", path, err)
	}
	return nil
}

// Activate replaces the active configuration with the named alternative.
func (c *Configurations) Activate(name string) error {
	path, err := c.alternativePath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read client configuration %q: %w", name, err)
	}
	configuration, err := decode(data)
	if err != nil {
		return fmt.Errorf("invalid client configuration %q: %w", name, err)
	}
	data, err = json.MarshalIndent(configuration, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal client configuration %q: %w", name, err)
	}
	if err := os.WriteFile(c.activePath, data, 0600); err != nil {
		return fmt.Errorf("failed to activate client configuration %q: %w", name, err)
	}
	return nil
}

// Delete removes the named alternative.
func (c *Configurations) Delete(name string) error {
	path, err := c.alternativePath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (c *Configurations) alternativePath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) ||
		name == "." || name == ".." || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("invalid configuration name %q", name)
	}
	return c.activePath + "." + name, nil
}

func decode(data []byte) (Configuration, error) {
	var configuration Configuration
	if err := json.Unmarshal(data, &configuration); err != nil {
		return Configuration{}, err
	}
	configuration.applyDefaults()
	if err := validate(configuration); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}
