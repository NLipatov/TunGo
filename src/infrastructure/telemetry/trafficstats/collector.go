package trafficstats

import (
	"context"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	RXBytesTotal uint64
	TXBytesTotal uint64
	RXRate       uint64 // bytes/sec
	TXRate       uint64 // bytes/sec
}

const HotPathFlushThresholdBytes uint64 = 64 * 1024

type Collector struct {
	rxBytesTotal atomic.Uint64
	txBytesTotal atomic.Uint64
	rxRate       atomic.Uint64
	txRate       atomic.Uint64

	sampleInterval time.Duration
	emaAlpha       float64

	// accessed only from the single sampler goroutine in Start()
	lastRX  uint64
	lastTX  uint64
	rxEMA   float64
	txEMA   float64
	started atomic.Bool
}

func NewCollector(sampleInterval time.Duration, emaAlpha float64) *Collector {
	if sampleInterval <= 0 {
		sampleInterval = time.Second
	}
	if emaAlpha < 0 {
		emaAlpha = 0
	}
	if emaAlpha > 1 {
		emaAlpha = 1
	}
	return &Collector{
		sampleInterval: sampleInterval,
		emaAlpha:       emaAlpha,
	}
}

func (c *Collector) Start(ctx context.Context) {
	if !c.started.CompareAndSwap(false, true) {
		return
	}

	ticker := time.NewTicker(c.sampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.updateRates(c.sampleInterval)
		}
	}
}

func (c *Collector) AddRX(bytes int) {
	if bytes <= 0 {
		return
	}
	c.AddRXBytes(uint64(bytes))
}

func (c *Collector) AddTX(bytes int) {
	if bytes <= 0 {
		return
	}
	c.AddTXBytes(uint64(bytes))
}

// AddRXBytes is allocation-free and intended for hot paths.
func (c *Collector) AddRXBytes(bytes uint64) {
	if bytes == 0 {
		return
	}
	c.rxBytesTotal.Add(bytes)
}

// AddTXBytes is allocation-free and intended for hot paths.
func (c *Collector) AddTXBytes(bytes uint64) {
	if bytes == 0 {
		return
	}
	c.txBytesTotal.Add(bytes)
}

func (c *Collector) Snapshot() Snapshot {
	return Snapshot{
		RXBytesTotal: c.rxBytesTotal.Load(),
		TXBytesTotal: c.txBytesTotal.Load(),
		RXRate:       c.rxRate.Load(),
		TXRate:       c.txRate.Load(),
	}
}

func (c *Collector) updateRates(interval time.Duration) {
	seconds := interval.Seconds()
	if seconds <= 0 {
		return
	}

	rxNow := c.rxBytesTotal.Load()
	txNow := c.txBytesTotal.Load()

	rxDelta := rxNow - c.lastRX
	txDelta := txNow - c.lastTX
	c.lastRX = rxNow
	c.lastTX = txNow

	rxPerSec := float64(rxDelta) / seconds
	txPerSec := float64(txDelta) / seconds

	if c.emaAlpha > 0 {
		if c.rxEMA == 0 {
			c.rxEMA = rxPerSec
		} else {
			c.rxEMA = c.emaAlpha*rxPerSec + (1-c.emaAlpha)*c.rxEMA
		}
		if c.txEMA == 0 {
			c.txEMA = txPerSec
		} else {
			c.txEMA = c.emaAlpha*txPerSec + (1-c.emaAlpha)*c.txEMA
		}
		rxPerSec = c.rxEMA
		txPerSec = c.txEMA
	}

	c.rxRate.Store(uint64(rxPerSec))
	c.txRate.Store(uint64(txPerSec))
}
