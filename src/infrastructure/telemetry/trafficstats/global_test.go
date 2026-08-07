package trafficstats

import (
	"testing"
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
