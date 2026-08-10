package trafficstats

import (
	"context"
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

func TestCollector_UpdateRates_WithEMA_TXSmoothingBranch(t *testing.T) {
	c := NewCollector(time.Second, 0.5)
	c.AddTX(1000)
	c.updateRates(time.Second) // 1000
	c.AddTX(3000)
	c.updateRates(time.Second) // raw 3000, ema 2000

	s := c.Snapshot()
	if s.TXRate < 1900 || s.TXRate > 2100 {
		t.Fatalf("expected smoothed TX around 2000, got %d", s.TXRate)
	}
}

func TestNewCollector_NormalizesInputs(t *testing.T) {
	c := NewCollector(0, -1)
	if c.sampleInterval != time.Second {
		t.Fatalf("expected default interval 1s, got %v", c.sampleInterval)
	}
	if c.emaAlpha != 0 {
		t.Fatalf("expected emaAlpha clamped to 0, got %v", c.emaAlpha)
	}

	c2 := NewCollector(time.Second, 2)
	if c2.emaAlpha != 1 {
		t.Fatalf("expected emaAlpha clamped to 1, got %v", c2.emaAlpha)
	}
}

func TestCollector_AddHelpers_IgnoreNonPositiveAndZeroBytes(t *testing.T) {
	c := NewCollector(time.Second, 0)
	c.AddRX(0)
	c.AddRX(-10)
	c.AddTX(0)
	c.AddTX(-10)
	c.AddRXBytes(0)
	c.AddTXBytes(0)
	s := c.Snapshot()
	if s.RXBytesTotal != 0 || s.TXBytesTotal != 0 {
		t.Fatalf("expected totals to stay zero, got %+v", s)
	}
}

func TestCollector_UpdateRates_ZeroIntervalDoesNothing(t *testing.T) {
	c := NewCollector(time.Second, 0)
	c.AddRX(512)
	c.AddTX(256)
	c.updateRates(0)
	s := c.Snapshot()
	if s.RXRate != 0 || s.TXRate != 0 {
		t.Fatalf("expected rates to remain zero, got %+v", s)
	}
}

func TestCollector_Start_UpdatesRateAndStopsOnCancel(t *testing.T) {
	c := NewCollector(20*time.Millisecond, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Start(ctx)
		close(done)
	}()

	stopTraffic := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopTraffic:
				return
			case <-ticker.C:
				c.AddRXBytes(4096)
				c.AddTXBytes(2048)
			}
		}
	}()

	deadline := time.Now().Add(400 * time.Millisecond)
	var s Snapshot
	for time.Now().Before(deadline) {
		s = c.Snapshot()
		if s.RXRate != 0 && s.TXRate != 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s.RXRate == 0 || s.TXRate == 0 {
		t.Fatalf("expected non-zero rates after ticker update, got %+v", s)
	}

	close(stopTraffic)
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("collector did not stop after context cancellation")
	}
}

func TestCollector_Start_IsIdempotent(t *testing.T) {
	c := NewCollector(10*time.Millisecond, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.Start(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	c.Start(ctx) // second call should return immediately because started=true
	cancel()
	<-done
}
