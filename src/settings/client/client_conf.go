package client

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"tungo/settings"
)

type Conf struct {
	TCPSettings               settings.ConnectionSettings `json:"TCPSettings"`
	UDPSettings               settings.ConnectionSettings `json:"UDPSettings"`
	Ed25519PublicKey          ed25519.PublicKey           `json:"Ed25519PublicKey"`
	TCPWriteChannelBufferSize int32                       `json:"TCPWriteChannelBufferSize"`
	UDPNonceRingBufferSize    int                         `json:"UDPNonceRingBufferSize"`
	Protocol                  settings.Protocol           `json:"Protocol"`
}

func (s *Conf) Read() (*Conf, error) {
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

	return s, nil
}

func getServerConfPath() (string, error) {
	execPath, err := os.Getwd()
	if err != nil {
		return "", err
	}
	settingsPath := filepath.Join(filepath.Dir(execPath), "src", "settings", "client", "conf.json")
	return settingsPath, nil
}
