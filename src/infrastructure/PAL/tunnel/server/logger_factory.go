package server

import (
	"log/slog"
)

type loggerFactory interface {
	newLogger() *slog.Logger
}

type defaultLoggerFactory struct {
}

func newDefaultLoggerFactory() loggerFactory {
	return &defaultLoggerFactory{}
}

func (factory *defaultLoggerFactory) newLogger() *slog.Logger {
	return slog.Default()
}
