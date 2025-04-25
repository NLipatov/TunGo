package server_configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	if _, statErr := os.Stat(c.path); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration file does not exist: %s", c.path)
		}

		return nil, fmt.Errorf("configuration file not found: %s", c.path)
	}

	fileBytes, readFileErr := os.ReadFile(c.path)
	if readFileErr != nil {
		return nil, fmt.Errorf("configuration file (%s) is unreadable: %s", c.path, readFileErr)
	}

	var configuration Configuration
	deserializationErr := json.Unmarshal(fileBytes, &configuration)
	if deserializationErr != nil {
		return nil, fmt.Errorf("configuration file (%s) is invalid: %s", c.path, deserializationErr)
	}

	c.setEnvServerAddress(&configuration)
	c.setEnvEnabledProtocols(&configuration)

	return &configuration, nil
}

func (c *reader) setEnvServerAddress(conf *Configuration) {
	sIP := os.Getenv("ServerIP")
	if sIP != "" {
		conf.FallbackServerAddress = sIP
	}
}

func (c *reader) setEnvEnabledProtocols(conf *Configuration) {
	envUdp := os.Getenv("EnableUDP")
	envTCP := os.Getenv("EnableTCP")

	if envUdp != "" {
		eUDPBool, parseErr := strconv.ParseBool(envUdp)
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
}
