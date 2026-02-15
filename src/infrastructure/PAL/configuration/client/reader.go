package client

import (
	"encoding/json"
	"fmt"
	"os"
)

type reader struct {
	path string
}

func newReader(path string) *reader {
	return &reader{
		path: path,
	}
}

func (c *reader) read() (*Configuration, error) {
	var configuration Configuration
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &configuration)
	if err != nil {
		return nil, err
	}

	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("invalid client configuration (%s): %w", c.path, err)
	}

	return &configuration, nil
}
