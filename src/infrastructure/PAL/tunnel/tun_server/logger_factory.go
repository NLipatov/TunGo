package tun_server

import (
	application "tungo/application/logging"
	"tungo/infrastructure/logging"
)

type loggerFactory interface {
	newLogger() application.Logger
}

type defaultLoggerFactory struct {
}

func newDefaultLoggerFactory() loggerFactory {
	return &defaultLoggerFactory{}
}

func (factory *defaultLoggerFactory) newLogger() application.Logger {
	return logging.NewLogLogger()
}
