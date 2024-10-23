package server

import (
	"crypto/ed25519"
	"encoding/json"
	"etha-tunnel/settings"
	"os"
	"path/filepath"
)

type Conf struct {
	TCPSettings           settings.ConnectionSettings `json:"TCPSettings"`
	UDPSettings           settings.ConnectionSettings `json:"UDPSettings"`
	FallbackServerAddress string                      `json:"FallbackServerAddress"`
	Ed25519PublicKey      ed25519.PublicKey           `json:"Ed25519PublicKey"`
	Ed25519PrivateKey     ed25519.PrivateKey          `json:"Ed25519PrivateKey"`
	ClientCounter         int                         `json:"ClientCounter"`
}

func (s *Conf) InsertEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error {
	currentConf, err := s.Read()
	currentConf.Ed25519PublicKey = public
	currentConf.Ed25519PrivateKey = private

	err = s.RewriteConf()
	if err != nil {
		return err
	}

	return nil
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

func (s *Conf) RewriteConf() error {
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
	defer file.Close()

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
