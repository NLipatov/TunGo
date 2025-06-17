package logging

import (
	"log"
	"tungo/application"
)

type LogLogger struct {
}

func NewLogLogger() application.Logger {
	return &LogLogger{}
}

func (l LogLogger) Printf(format string, v ...any) {
	log.Printf(format, v...)
}
