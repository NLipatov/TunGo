package trafficstats

import (
	"testing"
	"time"
)

func TestCollector_UpdateRates(t *testing.T) {
	c := NewCollector(time.Second, 0)
	c.AddRX(2048)
	c.AddTX(1024)

	c.updateRates(time.Second)
	s := c.Snapshot()
	if s.RXRate != 2048 {
		t.Fatalf("expected RXRate 2048, got %d", s.RXRate)
	}
	if s.TXRate != 1024 {
		t.Fatalf("expected TXRate 1024, got %d", s.TXRate)
	}
}

func TestCollector_UpdateRates_WithEMA(t *testing.T) {
	c := NewCollector(time.Second, 0.5)
	c.AddRX(1000)
	c.updateRates(time.Second) // 1000
	c.AddRX(3000)
	c.updateRates(time.Second) // raw 3000, ema 2000

	s := c.Snapshot()
	if s.RXRate < 1900 || s.RXRate > 2100 {
		t.Fatalf("expected smoothed RX around 2000, got %d", s.RXRate)
	}
}
