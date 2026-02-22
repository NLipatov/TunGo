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
	TailInto(dst []string, limit int) int
}

type RuntimeLogChangeFeed interface {
	RuntimeLogFeed
	Changes() <-chan struct{}
}

type RuntimeLogBuffer struct {
	mu       sync.Mutex
	capacity int
	lines    []string // pre-allocated at full capacity
	head     int      // next write position
	count    int      // valid entries (0..capacity)
	partial  string
	changes  chan struct{}
}

var (
	globalRuntimeLogMu      sync.Mutex
	globalRuntimeLogBuffer  *RuntimeLogBuffer
	globalRuntimeLogRestore func()
)

func NewRuntimeLogBuffer(capacity int) *RuntimeLogBuffer {
	if capacity <= 0 {
		capacity = defaultRuntimeLogCapacity
	}
	return &RuntimeLogBuffer{
		capacity: capacity,
		lines:    make([]string, capacity),
		changes:  make(chan struct{}, 1),
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

	if limit <= 0 || b.count == 0 {
		return nil
	}
	n := minInt(b.count, limit)
	out := make([]string, n)
	b.copyTailLocked(out, n)
	return out
}

func (b *RuntimeLogBuffer) TailInto(dst []string, limit int) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if limit <= 0 || len(dst) == 0 || b.count == 0 {
		return 0
	}
	if limit > len(dst) {
		limit = len(dst)
	}
	n := minInt(b.count, limit)
	b.copyTailLocked(dst, n)
	return n
}

func (b *RuntimeLogBuffer) appendLineLocked(line string) {
	b.lines[b.head] = line
	b.head = (b.head + 1) % b.capacity
	if b.count < b.capacity {
		b.count++
	}
	b.signalChangeLocked()
}

func (b *RuntimeLogBuffer) copyTailLocked(dst []string, n int) {
	start := (b.head - n + b.capacity) % b.capacity
	if start+n <= b.capacity {
		copy(dst, b.lines[start:start+n])
	} else {
		first := b.capacity - start
		copy(dst, b.lines[start:])
		copy(dst[first:], b.lines[:n-first])
	}
}

func (b *RuntimeLogBuffer) signalChangeLocked() {
	select {
	case b.changes <- struct{}{}:
	default:
	}
}

func (b *RuntimeLogBuffer) Changes() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.changes
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

func EnableGlobalRuntimeLogCapture(capacity int) {
	globalRuntimeLogMu.Lock()
	defer globalRuntimeLogMu.Unlock()

	if globalRuntimeLogBuffer != nil {
		return
	}

	buffer := NewRuntimeLogBuffer(capacity)
	previousWriter := log.Writer()
	log.SetOutput(io.Writer(buffer))

	globalRuntimeLogBuffer = buffer
	globalRuntimeLogRestore = func() {
		log.SetOutput(previousWriter)
	}
}

func DisableGlobalRuntimeLogCapture() {
	globalRuntimeLogMu.Lock()
	defer globalRuntimeLogMu.Unlock()

	if globalRuntimeLogRestore != nil {
		globalRuntimeLogRestore()
	}
	globalRuntimeLogRestore = nil
	globalRuntimeLogBuffer = nil
}

func (b *RuntimeLogBuffer) WriteSeparator() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.appendLineLocked("---")
}

func GlobalRuntimeLogWriteSeparator() {
	globalRuntimeLogMu.Lock()
	buf := globalRuntimeLogBuffer
	globalRuntimeLogMu.Unlock()
	if buf != nil {
		buf.WriteSeparator()
	}
}

func GlobalRuntimeLogFeed() RuntimeLogFeed {
	globalRuntimeLogMu.Lock()
	defer globalRuntimeLogMu.Unlock()
	if globalRuntimeLogBuffer == nil {
		return nil
	}
	return globalRuntimeLogBuffer
}
