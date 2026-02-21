package trafficstats

import (
	"testing"
	"time"
)

func TestGlobal_NilCollectorDefaults(t *testing.T) {
	SetGlobal(nil)
	if Global() != nil {
		t.Fatal("expected nil global collector")
	}
	s := SnapshotGlobal()
	if s != (Snapshot{}) {
		t.Fatalf("expected empty snapshot, got %+v", s)
	}
}

func TestGlobal_AddHelpers_UpdateTotals(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	t.Cleanup(func() { SetGlobal(nil) })

	AddRX(-1)
	AddTX(-1)
	AddRX(0)
	AddTX(0)
	AddRXBytes(0)
	AddTXBytes(0)
	AddRX(1500)
	AddTX(900)
	AddRXBytes(600)
	AddTXBytes(100)

	s := SnapshotGlobal()
	if s.RXBytesTotal != 2100 {
		t.Fatalf("expected RX total 2100, got %d", s.RXBytesTotal)
	}
	if s.TXBytesTotal != 1000 {
		t.Fatalf("expected TX total 1000, got %d", s.TXBytesTotal)
	}
}
