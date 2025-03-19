package server_json_file_configuration

import (
	"encoding/json"
	"os"
	"tungo/settings/server"
)

type writer struct {
	resolver pathResolver
}

func newWriter(resolver pathResolver) *writer {
	return &writer{
		resolver: resolver,
	}
}

func (w *writer) Write(configuration server.Configuration) error {
	confPath, err := w.resolver.resolve()
	if err != nil {
		return err
	}

	jsonContent, err := json.MarshalIndent(configuration, "", "  ")
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
