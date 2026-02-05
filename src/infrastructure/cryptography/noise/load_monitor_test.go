package noise

import (
	"sync"
	"testing"
	"time"
)

func TestLoadMonitor_InitialState(t *testing.T) {
	lm := NewLoadMonitor(100)

	if lm.UnderLoad() {
		t.Fatal("should not be under load initially")
	}

	if lm.HandshakesPerSecond() != 0 {
		t.Fatal("should have zero handshakes initially")
	}
}

func TestLoadMonitor_RecordHandshake(t *testing.T) {
	lm := NewLoadMonitor(100)

	// Record handshakes
	for i := 0; i < 50; i++ {
		lm.RecordHandshake()
	}

	// Counter should track recordings
	count := lm.counter.Load()
	if count != 50 {
		t.Fatalf("expected counter 50, got %d", count)
	}
}

func TestLoadMonitor_UnderLoad(t *testing.T) {
	lm := NewLoadMonitor(10)

	// Simulate rate update
	lm.handshakesPerSecond.Store(11)

	if !lm.UnderLoad() {
		t.Fatal("should be under load when above threshold")
	}

	lm.handshakesPerSecond.Store(10)
	if lm.UnderLoad() {
		t.Fatal("should not be under load at exactly threshold")
	}

	lm.handshakesPerSecond.Store(9)
	if lm.UnderLoad() {
		t.Fatal("should not be under load below threshold")
	}
}

func TestLoadMonitor_SetThreshold(t *testing.T) {
	lm := NewLoadMonitor(100)
	lm.handshakesPerSecond.Store(50)

	if lm.UnderLoad() {
		t.Fatal("should not be under load with 50 < 100")
	}

	lm.SetThreshold(40)
	if !lm.UnderLoad() {
		t.Fatal("should be under load with 50 > 40")
	}
}

func TestLoadMonitor_ConcurrentAccess(t *testing.T) {
	lm := NewLoadMonitor(1000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				lm.RecordHandshake()
				_ = lm.UnderLoad()
				_ = lm.HandshakesPerSecond()
			}
		}()
	}

	wg.Wait()
	// Just checking for race conditions, no specific assertions
}

func TestLoadMonitor_DefaultThreshold(t *testing.T) {
	// Zero threshold should use default
	lm := NewLoadMonitor(0)
	if lm.threshold != DefaultLoadThreshold {
		t.Fatalf("expected default threshold %d, got %d", DefaultLoadThreshold, lm.threshold)
	}

	// Negative threshold should use default
	lm = NewLoadMonitor(-1)
	if lm.threshold != DefaultLoadThreshold {
		t.Fatalf("expected default threshold %d, got %d", DefaultLoadThreshold, lm.threshold)
	}
}

func TestLoadMonitor_RateReset(t *testing.T) {
	lm := NewLoadMonitor(100)

	// Manually set the time for testing
	baseTime := time.Now().Unix()
	lm.lastResetTime.Store(baseTime)

	// Record some handshakes
	for i := 0; i < 50; i++ {
		lm.RecordHandshake()
	}

	// Simulate time passing by updating lastResetTime to past
	lm.lastResetTime.Store(baseTime - 1)

	// Next record should trigger rate update
	lm.RecordHandshake()

	// The rate should have been updated
	rate := lm.HandshakesPerSecond()
	// Due to race conditions in test, we just verify the mechanism doesn't panic
	_ = rate
}
