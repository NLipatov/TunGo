package client

import (
	"encoding/json"
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

	return &configuration, nil
}
