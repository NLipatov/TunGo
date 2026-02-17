package bubble_tea

import (
	"io"
	"log"
	"strings"
	"sync"
)

const defaultRuntimeLogCapacity = 256

type RuntimeLogFeed interface {
	Tail(limit int) []string
}

type RuntimeLogBuffer struct {
	mu       sync.Mutex
	capacity int
	lines    []string
	partial  string
}

func NewRuntimeLogBuffer(capacity int) *RuntimeLogBuffer {
	if capacity <= 0 {
		capacity = defaultRuntimeLogCapacity
	}
	return &RuntimeLogBuffer{
		capacity: capacity,
		lines:    make([]string, 0, capacity),
	}
}

func (b *RuntimeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chunk := string(p)
	for len(chunk) > 0 {
		newlineIdx := strings.IndexByte(chunk, '\n')
		if newlineIdx < 0 {
			b.partial += chunk
			break
		}
		b.partial += chunk[:newlineIdx]
		b.appendLineLocked(strings.TrimRight(b.partial, "\r"))
		b.partial = ""
		chunk = chunk[newlineIdx+1:]
	}
	return len(p), nil
}

func (b *RuntimeLogBuffer) Tail(limit int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if limit <= 0 || len(b.lines) == 0 {
		return nil
	}
	start := 0
	if len(b.lines) > limit {
		start = len(b.lines) - limit
	}
	out := make([]string, len(b.lines)-start)
	copy(out, b.lines[start:])
	return out
}

func (b *RuntimeLogBuffer) appendLineLocked(line string) {
	if line == "" {
		return
	}
	if len(b.lines) < b.capacity {
		b.lines = append(b.lines, line)
		return
	}
	copy(b.lines, b.lines[1:])
	b.lines[len(b.lines)-1] = line
}

func RedirectStandardLoggerToBuffer(buffer *RuntimeLogBuffer) func() {
	if buffer == nil {
		return func() {}
	}
	previousWriter := log.Writer()
	log.SetOutput(io.Writer(buffer))
	return func() {
		log.SetOutput(previousWriter)
	}
}
