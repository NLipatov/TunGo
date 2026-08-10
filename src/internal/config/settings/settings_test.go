package settings

import (
	"encoding/json"
	"testing"
)

func TestSettings_JSON_IntPort(t *testing.T) {
	data := []byte(`{"Port": 8080}`)
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", s.Port)
	}
}

func TestSettings_JSON_MissingPort(t *testing.T) {
	data := []byte(`{"MTU": 1500}`)
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Port != 0 {
		t.Fatalf("expected port 0, got %d", s.Port)
	}
	if s.MTU != 1500 {
		t.Fatalf("expected MTU 1500, got %d", s.MTU)
	}
}
