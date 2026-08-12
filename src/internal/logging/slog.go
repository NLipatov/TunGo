package logging

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	outputMu sync.RWMutex
	output   io.Writer = os.Stderr
)

type dynamicWriter struct{}

func (dynamicWriter) Write(p []byte) (int, error) {
	outputMu.RLock()
	w := output
	outputMu.RUnlock()
	return w.Write(p)
}

// Writer returns an io.Writer that always forwards to the current logging sink.
func Writer() io.Writer {
	return dynamicWriter{}
}

// SetOutput replaces the shared logging sink and returns the previous writer.
func SetOutput(w io.Writer) io.Writer {
	outputMu.Lock()
	defer outputMu.Unlock()

	prev := output
	if w == nil {
		output = os.Stderr
		return prev
	}
	output = w
	return prev
}

func CurrentOutput() io.Writer {
	outputMu.RLock()
	defer outputMu.RUnlock()
	return output
}

func NewLogger(level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewTextHandler(Writer(), &slog.HandlerOptions{
		Level: level,
	}))
}
