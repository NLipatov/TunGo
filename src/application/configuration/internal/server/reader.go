package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Reader interface {
	read() (*Configuration, error)
}

type defaultReader struct {
	path string
}

func newDefaultReader(path string) *defaultReader {
	return &defaultReader{path: path}
}

func (c *defaultReader) read() (*Configuration, error) {
	fileBytes, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration file %q does not exist: %w", c.path, err)
		}
		return nil, fmt.Errorf("configuration file %q is unreadable: %w", c.path, err)
	}

	var configuration Configuration
	if err := json.Unmarshal(fileBytes, &configuration); err != nil {
		return nil, fmt.Errorf("configuration file %q is invalid: %w", c.path, err)
	}

	c.setEnvServerAddress(&configuration)
	c.setEnvEnabledProtocols(&configuration)
	configuration.ApplyServerDefaults()
	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("configuration file %q is invalid: %w", c.path, err)
	}

	return &configuration, nil
}

func (c *defaultReader) setEnvServerAddress(conf *Configuration) {
	sIP := os.Getenv("ServerIP")
	if sIP != "" {
		conf.FallbackServerAddress = sIP
	}
}

func (c *defaultReader) setEnvEnabledProtocols(conf *Configuration) {
	envUDP := os.Getenv("EnableUDP")
	envTCP := os.Getenv("EnableTCP")
	envWS := os.Getenv("EnableWS")

	if envUDP != "" {
		eUDPBool, parseErr := strconv.ParseBool(envUDP)
		if parseErr == nil {
			conf.EnableUDP = eUDPBool
		}
	}

	if envTCP != "" {
		eTCPBool, parseErr := strconv.ParseBool(envTCP)
		if parseErr == nil {
			conf.EnableTCP = eTCPBool
		}
	}

	if envWS != "" {
		eWSBool, parseErr := strconv.ParseBool(envWS)
		if parseErr == nil {
			conf.EnableWS = eWSBool
		}
	}
}
