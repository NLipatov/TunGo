package trafficstats

import "sync/atomic"

var globalCollector atomic.Pointer[Collector]

func SetGlobal(collector *Collector) {
	globalCollector.Store(collector)
}

func Global() *Collector {
	return globalCollector.Load()
}

func SnapshotGlobal() Snapshot {
	if collector := globalCollector.Load(); collector != nil {
		return collector.Snapshot()
	}
	return Snapshot{}
}

func AddRX(bytes int) {
	if bytes <= 0 {
		return
	}
	AddRXBytes(uint64(bytes))
}

func AddTX(bytes int) {
	if bytes <= 0 {
		return
	}
	AddTXBytes(uint64(bytes))
}

// AddRXBytes is allocation-free and intended for hot paths.
func AddRXBytes(bytes uint64) {
	if bytes == 0 {
		return
	}
	if collector := globalCollector.Load(); collector != nil {
		collector.rxBytesTotal.Add(bytes)
	}
}

// AddTXBytes is allocation-free and intended for hot paths.
func AddTXBytes(bytes uint64) {
	if bytes == 0 {
		return
	}
	if collector := globalCollector.Load(); collector != nil {
		collector.txBytesTotal.Add(bytes)
	}
}
