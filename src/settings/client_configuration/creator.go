package client_configuration

import (
	"encoding/json"
	"fmt"
	"os"
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
	serialized, serializedErr := json.MarshalIndent(configuration, "", "\t")
	if serializedErr != nil {
		return serializedErr
	}

	defaultConfPath, defaultConfPathErr := d.resolver.Resolve()
	if defaultConfPathErr != nil {
		return defaultConfPathErr
	}

	confPath := fmt.Sprintf("%s.%s", defaultConfPath, name)
	writeErr := os.WriteFile(confPath, serialized, 0600)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
