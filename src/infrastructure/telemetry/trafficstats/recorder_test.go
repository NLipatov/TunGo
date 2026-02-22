package trafficstats

import (
	"testing"
	"time"
)

func TestRecorder_FlushDrainsPending(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	defer SetGlobal(nil)

	rec := NewRecorder()
	rec.RecordRX(100)
	rec.RecordTX(200)

	// Not yet flushed — below threshold
	snap := c.Snapshot()
	if snap.RXBytesTotal != 0 || snap.TXBytesTotal != 0 {
		t.Fatalf("expected zeros before flush, got rx=%d tx=%d", snap.RXBytesTotal, snap.TXBytesTotal)
	}

	rec.Flush()

	snap = c.Snapshot()
	if snap.RXBytesTotal != 100 {
		t.Fatalf("expected RX=100 after flush, got %d", snap.RXBytesTotal)
	}
	if snap.TXBytesTotal != 200 {
		t.Fatalf("expected TX=200 after flush, got %d", snap.TXBytesTotal)
	}
}

func TestRecorder_AutoFlushOnThreshold(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	defer SetGlobal(nil)

	rec := NewRecorder()
	rec.RecordRX(HotPathFlushThresholdBytes)

	snap := c.Snapshot()
	if snap.RXBytesTotal != HotPathFlushThresholdBytes {
		t.Fatalf("expected auto-flush at threshold, got %d", snap.RXBytesTotal)
	}
}

func TestRecorder_NilCollector_IsNoop(t *testing.T) {
	SetGlobal(nil)
	rec := NewRecorder()
	rec.RecordRX(999)
	rec.RecordTX(999)
	rec.Flush() // must not panic
}

func TestRecorder_DoubleFlush(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	defer SetGlobal(nil)

	rec := NewRecorder()
	rec.RecordRX(42)
	rec.Flush()
	rec.Flush()

	snap := c.Snapshot()
	if snap.RXBytesTotal != 42 {
		t.Fatalf("expected 42 after double flush, got %d", snap.RXBytesTotal)
	}
}

func TestRecorder_AutoFlushTXOnThreshold(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	defer SetGlobal(nil)

	rec := NewRecorder()
	rec.RecordTX(HotPathFlushThresholdBytes)

	snap := c.Snapshot()
	if snap.TXBytesTotal != HotPathFlushThresholdBytes {
		t.Fatalf("expected TX auto-flush at threshold, got %d", snap.TXBytesTotal)
	}
}

func TestRecorder_ZeroBytes(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	defer SetGlobal(nil)

	rec := NewRecorder()
	rec.RecordRX(0)
	rec.RecordTX(0)
	rec.Flush()

	snap := c.Snapshot()
	if snap.RXBytesTotal != 0 || snap.TXBytesTotal != 0 {
		t.Fatalf("expected zeros, got rx=%d tx=%d", snap.RXBytesTotal, snap.TXBytesTotal)
	}
}
