package logging

import (
	"log"
	"tungo/application/logging"
)

type LogLogger struct {
}

func NewLogLogger() logging.Logger {
	return &LogLogger{}
}

func (l LogLogger) Printf(format string, v ...any) {
	log.Printf(format, v...)
}
