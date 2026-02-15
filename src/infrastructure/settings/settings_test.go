package settings

import (
	"encoding/json"
	"testing"
)

func TestSettings_UnmarshalJSON_IntPort(t *testing.T) {
	data := []byte(`{"Port": 8080}`)
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", s.Port)
	}
}

func TestSettings_UnmarshalJSON_StringPort(t *testing.T) {
	data := []byte(`{"Port": "9090"}`)
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", s.Port)
	}
}

func TestSettings_UnmarshalJSON_InvalidPortString(t *testing.T) {
	data := []byte(`{"Port": "not-a-number"}`)
	var s Settings
	if err := json.Unmarshal(data, &s); err == nil {
		t.Fatal("expected error for non-numeric port string")
	}
}

func TestSettings_UnmarshalJSON_MissingPort(t *testing.T) {
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
