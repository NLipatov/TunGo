package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Creator interface {
	Create(configuration Configuration, name string) error
}

type DefaultCreator struct {
	resolver Resolver
}

func NewDefaultCreator(resolver Resolver) Creator {
	return &DefaultCreator{
		resolver: resolver,
	}
}

func (d *DefaultCreator) Create(configuration Configuration, name string) error {
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.ContainsAny(name, "\x00") {
		return fmt.Errorf("invalid configuration name %q: must not contain path separators", name)
	}

	serialized, serializedErr := json.MarshalIndent(configuration, "", "\t")
	if serializedErr != nil {
		return serializedErr
	}

	defaultConfPath, defaultConfPathErr := d.resolver.Resolve()
	if defaultConfPathErr != nil {
		return defaultConfPathErr
	}

	confPath := fmt.Sprintf("%s.%s", defaultConfPath, name)
	mkdirErr := os.MkdirAll(filepath.Dir(confPath), 0700)
	if mkdirErr != nil {
		return mkdirErr
	}

	writeErr := os.WriteFile(confPath, serialized, 0600)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
