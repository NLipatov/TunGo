package trafficstats

// Recorder batches RX/TX byte counts and flushes them to the global
// Collector when the accumulated total reaches HotPathFlushThresholdBytes.
//
// A Recorder is NOT safe for concurrent use — create one per goroutine.
// Call Flush (typically via defer) to drain any remaining bytes.
type Recorder struct {
	collector *Collector
	pendingRX uint64
	pendingTX uint64
}

// NewRecorder returns a Recorder bound to the current Global() collector.
// If the global collector is nil, all Record/Flush calls are no-ops.
func NewRecorder() Recorder {
	return Recorder{collector: Global()}
}

func (r *Recorder) RecordRX(bytes uint64) {
	if r.collector == nil || bytes == 0 {
		return
	}
	r.pendingRX += bytes
	if r.pendingRX >= HotPathFlushThresholdBytes {
		r.collector.AddRXBytes(r.pendingRX)
		r.pendingRX = 0
	}
}

func (r *Recorder) RecordTX(bytes uint64) {
	if r.collector == nil || bytes == 0 {
		return
	}
	r.pendingTX += bytes
	if r.pendingTX >= HotPathFlushThresholdBytes {
		r.collector.AddTXBytes(r.pendingTX)
		r.pendingTX = 0
	}
}

func (r *Recorder) Flush() {
	if r.collector == nil {
		return
	}
	if r.pendingRX != 0 {
		r.collector.AddRXBytes(r.pendingRX)
		r.pendingRX = 0
	}
	if r.pendingTX != 0 {
		r.collector.AddTXBytes(r.pendingTX)
		r.pendingTX = 0
	}
}
