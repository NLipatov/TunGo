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
