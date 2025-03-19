package server_json_file_configuration

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"tungo/settings/server"
)

type reader struct {
	path string
}

func newReader(path string) *reader {
	return &reader{
		path: path,
	}
}

func (c *reader) read() (*server.Configuration, error) {
	if !c.fileExists(c.path) {
		return nil, fmt.Errorf("configuration file not found: %s", c.path)
	}

	fileBytes, readFileErr := os.ReadFile(c.path)
	if readFileErr != nil {
		return nil, fmt.Errorf("configuration file (%s) is unreadable: %s", c.path, readFileErr)
	}

	var configuration server.Configuration
	deserializationErr := json.Unmarshal(fileBytes, &configuration)
	if deserializationErr != nil {
		return nil, fmt.Errorf("configuration file (%s) is invalid: %s", c.path, deserializationErr)
	}

	c.setEnvServerAddress(&configuration)
	c.setEnvEnabledProtocols(&configuration)
	c.setEnvUDPNonceRingBufferSize(&configuration)

	return &configuration, nil
}

func (c *reader) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (c *reader) setEnvServerAddress(conf *server.Configuration) {
	sIP := os.Getenv("ServerIP")
	if sIP != "" {
		conf.FallbackServerAddress = sIP
	}
}

func (c *reader) setEnvEnabledProtocols(conf *server.Configuration) {
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

func (c *reader) setEnvUDPNonceRingBufferSize(conf *server.Configuration) {
	envRBSize := os.Getenv("UDPNonceRingBufferSize")

	if envRBSize != "" {
		size, parseErr := strconv.Atoi(envRBSize)
		if parseErr == nil {
			conf.UDPNonceRingBufferSize = size
		}
	}
}
