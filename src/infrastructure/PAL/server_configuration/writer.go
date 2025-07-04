package server_configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Writer interface {
	Write(data interface{}) error
}

type defaultWriter struct {
	path string
}

func newDefaultWriter(path string) *defaultWriter {
	return &defaultWriter{
		path: path,
	}
}

func (w *defaultWriter) Write(data interface{}) error {
	jsonContent, jsonContentErr := json.MarshalIndent(data, "", "\t")
	if jsonContentErr != nil {
		return jsonContentErr
	}

	dir := filepath.Dir(w.path)
	mkdirErr := os.MkdirAll(dir, 0700)
	if mkdirErr != nil {
		return mkdirErr
	}

	file, fileErr := os.Create(w.path)
	if fileErr != nil {
		return fileErr
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	_, writeErr := file.Write(jsonContent)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
