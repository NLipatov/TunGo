package trafficstats

import "testing"

func TestFormatRate(t *testing.T) {
	if got := FormatRate(1200); got != "1.2 KiB/s" {
		t.Fatalf("unexpected rate format: %q", got)
	}
}

func TestFormatTotal(t *testing.T) {
	if got := FormatTotal(3 * 1024 * 1024); got != "3.0 MiB" {
		t.Fatalf("unexpected total format: %q", got)
	}
}

func TestFormatRateWithSystem_Bytes(t *testing.T) {
	if got := FormatRateWithSystem(1200, UnitSystemBytes); got != "1.2 KB/s" {
		t.Fatalf("unexpected decimal rate format: %q", got)
	}
}
