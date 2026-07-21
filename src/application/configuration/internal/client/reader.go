package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func read(path string) (*Configuration, error) {
	var configuration Configuration
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("client configuration %q does not exist: %w", path, err)
		}
		return nil, fmt.Errorf("failed to read client configuration %q: %w", path, err)
	}

	if err := json.Unmarshal(data, &configuration); err != nil {
		return nil, fmt.Errorf("invalid client configuration %q: %w", path, err)
	}

	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("invalid client configuration %q: %w", path, err)
	}

	return &configuration, nil
}
