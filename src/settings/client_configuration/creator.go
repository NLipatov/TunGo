package client_configuration

import (
	"encoding/json"
	"os"
)

type Creator interface {
	Create(configuration Configuration) error
}

type DefaultCreator struct {
	resolver Resolver
}

func NewDefaultCreator(resolver Resolver) Creator {
	return &DefaultCreator{
		resolver: resolver,
	}
}

func (d *DefaultCreator) Create(configuration Configuration) error {
	serialized, serializedErr := json.Marshal(configuration)
	if serializedErr != nil {
		return serializedErr
	}

	defaultConfPath, defaultConfPathErr := d.resolver.Resolve()
	if defaultConfPathErr != nil {
		return defaultConfPathErr
	}

	writeErr := os.WriteFile(defaultConfPath, serialized, 0600)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
