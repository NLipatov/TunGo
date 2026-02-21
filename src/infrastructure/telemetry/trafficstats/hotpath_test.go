package trafficstats

import (
	"testing"
	"time"
)

func TestHotPathAdd_NoAllocs(t *testing.T) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	t.Cleanup(func() { SetGlobal(nil) })

	allocs := testing.AllocsPerRun(1000, func() {
		AddRXBytes(1500)
		AddTXBytes(900)
	})
	if allocs != 0 {
		t.Fatalf("expected zero allocations in hot path, got %.2f", allocs)
	}
}

func BenchmarkHotPathAddBytes(b *testing.B) {
	c := NewCollector(time.Second, 0)
	SetGlobal(c)
	defer SetGlobal(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AddRXBytes(1500)
		AddTXBytes(900)
	}
}
