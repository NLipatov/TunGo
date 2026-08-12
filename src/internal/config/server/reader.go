package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Reader interface {
	read() (*Configuration, error)
}

type reader struct {
	path string
}

func newReader(path string) *reader {
	return &reader{path: path}
}

func (c *reader) read() (*Configuration, error) {
	configuration, err := c.readFromDisk()
	if err != nil {
		return nil, err
	}
	if err := Validate(configuration); err != nil {
		return nil, fmt.Errorf("configuration file %q is invalid: %w", c.path, err)
	}

	return &configuration, nil
}

func (c *reader) readFromDisk() (Configuration, error) {
	fileBytes, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Configuration{}, fmt.Errorf("configuration file %q does not exist: %w", c.path, err)
		}
		return Configuration{}, fmt.Errorf("configuration file %q is unreadable: %w", c.path, err)
	}
	type configurationWithDeprecatedFields struct {
		Configuration
		// FallbackServerAddress is deprecated. Host is used instead.
		FallbackServerAddress string `json:"FallbackServerAddress"`
	}
	var withDeprecated configurationWithDeprecatedFields
	if err := json.Unmarshal(fileBytes, &withDeprecated); err != nil {
		return Configuration{}, fmt.Errorf("configuration file %q is invalid: %w", c.path, err)
	}
	actual := withDeprecated.Configuration
	if strings.TrimSpace(actual.Host) == "" &&
		strings.TrimSpace(withDeprecated.FallbackServerAddress) != "" {
		actual.Host = strings.TrimSpace(withDeprecated.FallbackServerAddress)
	}
	c.setEnvServerHost(&actual)
	c.setEnvEnabledProtocols(&actual)
	actual.ApplyServerDefaults()
	return actual, nil
}

func (c *reader) setEnvServerHost(conf *Configuration) {
	host := strings.TrimSpace(os.Getenv("Host"))
	if len(host) > 0 {
		conf.Host = host
		return
	}
	// ServerIP is deprecated; keep it as a fallback for existing installations.
	serverIP := strings.TrimSpace(os.Getenv("ServerIP"))
	if len(serverIP) > 0 {
		conf.Host = serverIP
	}
}

func (c *reader) setEnvEnabledProtocols(conf *Configuration) {
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
