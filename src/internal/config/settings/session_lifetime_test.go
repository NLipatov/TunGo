package settings

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHumanReadableDuration_MarshalJSON(t *testing.T) {
	d := HumanReadableDuration(45 * time.Minute)
	b, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	expected := `"45m0s"`
	if string(b) != expected {
		t.Errorf("expected %s, got %s", expected, string(b))
	}
}

func TestHumanReadableDuration_UnmarshalJSON_Valid(t *testing.T) {
	var d HumanReadableDuration
	input := `"12h30m45s"`

	err := d.UnmarshalJSON([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	expected := 12*time.Hour + 30*time.Minute + 45*time.Second
	if time.Duration(d) != expected {
		t.Errorf("expected %v, got %v", expected, time.Duration(d))
	}
}

func TestHumanReadableDuration_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var d HumanReadableDuration
	input := `12345`

	err := d.UnmarshalJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid JSON string")
	}
}

func TestHumanReadableDuration_UnmarshalJSON_InvalidDuration(t *testing.T) {
	var d HumanReadableDuration
	input := `"notaduration"`

	err := d.UnmarshalJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid duration string")
	}
}

func TestSessionLifetime_JSONMarshalling(t *testing.T) {
	original := SessionLifetime{
		Ttl:             HumanReadableDuration(90 * time.Minute),
		CleanupInterval: HumanReadableDuration(15 * time.Minute),
	}

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded SessionLifetime
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Ttl != original.Ttl {
		t.Errorf("Ttl mismatch: expected %v, got %v", original.Ttl, decoded.Ttl)
	}

	if decoded.CleanupInterval != original.CleanupInterval {
		t.Errorf("CleanupInterval mismatch: expected %v, got %v", original.CleanupInterval, decoded.CleanupInterval)
	}
}
