package serverConfiguration

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Conf struct {
	IfName  string `json:"IfName"`
	IfIP    string `json:"IfIP"`
	TCPPort string `json:"TCPPort"`
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
	settingsPath := filepath.Join(filepath.Dir(execPath), "src", "settings", "serverConfiguration", "conf.json")
	return settingsPath, nil
}