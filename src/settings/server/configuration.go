package server

import (
	"crypto/ed25519"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"tungo/settings"
)

type Configuration struct {
	TCPSettings               settings.ConnectionSettings `json:"TCPSettings"`
	UDPSettings               settings.ConnectionSettings `json:"UDPSettings"`
	FallbackServerAddress     string                      `json:"FallbackServerAddress"`
	Ed25519PublicKey          ed25519.PublicKey           `json:"Ed25519PublicKey"`
	Ed25519PrivateKey         ed25519.PrivateKey          `json:"Ed25519PrivateKey"`
	ClientCounter             int                         `json:"ClientCounter"`
	TCPWriteChannelBufferSize int32                       `json:"TCPWriteChannelBufferSize"`
	UDPNonceRingBufferSize    int                         `json:"UDPNonceRingBufferSize"`
	EnableTCP                 bool                        `json:"EnableTCP"`
	EnableUDP                 bool                        `json:"EnableUDP"`
}

func NewDefaultConfiguration() Configuration {
	return Configuration{
		TCPSettings: settings.ConnectionSettings{
			InterfaceName:    "tcptun0",
			InterfaceIPCIDR:  "10.0.0.0/24",
			InterfaceAddress: "10.0.0.1",
			ConnectionIP:     "",
			Port:             "8080",
			MTU:              1472,
			Protocol:         0,
			Encryption:       0,
			DialTimeoutMs:    5000,
		},
		UDPSettings: settings.ConnectionSettings{
			InterfaceName:    "udptun0",
			InterfaceIPCIDR:  "10.0.1.0/24",
			InterfaceAddress: "10.0.1.1",
			ConnectionIP:     "",
			Port:             "9090",
			MTU:              1416,
			Protocol:         1,
			Encryption:       0,
			DialTimeoutMs:    5000,
		},
		FallbackServerAddress:     "",
		Ed25519PublicKey:          nil,
		Ed25519PrivateKey:         nil,
		ClientCounter:             0,
		TCPWriteChannelBufferSize: 1000,
		UDPNonceRingBufferSize:    100_000,
		EnableTCP:                 false,
		EnableUDP:                 true,
	}
}

func (s *Configuration) InsertEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error {
	currentConf, err := s.Read()
	if err != nil {
		log.Printf("failed to read configuration: %s", err)
	}

	currentConf.Ed25519PublicKey = public
	currentConf.Ed25519PrivateKey = private

	err = s.RewriteConf()
	if err != nil {
		return err
	}

	return nil
}

func (s *Configuration) Read() (*Configuration, error) {
	confPath, err := getServerConfPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		return nil, err
	}

	setEnvServerAddress(s)
	setEnvEnabledProtocols(s)
	setEnvUDPNonceRingBufferSize(s)

	return s, nil
}

func setEnvServerAddress(conf *Configuration) {
	sIP := os.Getenv("ServerIP")
	if sIP != "" {
		conf.FallbackServerAddress = sIP
	}
}

func setEnvEnabledProtocols(conf *Configuration) {
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

func setEnvUDPNonceRingBufferSize(conf *Configuration) {
	envRBSize := os.Getenv("UDPNonceRingBufferSize")

	if envRBSize != "" {
		size, parseErr := strconv.Atoi(envRBSize)
		if parseErr == nil {
			conf.UDPNonceRingBufferSize = size
		}
	}
}

func (s *Configuration) RewriteConf() error {
	confPath, err := getServerConfPath()
	if err != nil {
		return err
	}

	jsonContent, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	file, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	_, err = file.Write(jsonContent)
	if err != nil {
		return err
	}

	return nil
}

func getServerConfPath() (string, error) {
	execPath, err := os.Getwd()
	if err != nil {
		return "", err
	}
	settingsPath := filepath.Join(filepath.Dir(execPath), "src", "settings", "server", "conf.json")
	return settingsPath, nil
}
