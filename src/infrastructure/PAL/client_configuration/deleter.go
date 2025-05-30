package client_configuration

import (
	"os"
)

type Deleter interface {
	Delete(confName string) error
}

type DefaultDeleter struct {
	resolver Resolver
}

func NewDefaultDeleter(resolver Resolver) Deleter {
	return DefaultDeleter{
		resolver: resolver,
	}
}

func (d DefaultDeleter) Delete(confAbsPath string) error {
	return os.Remove(confAbsPath)
}
