package server_json_file_configuration

import (
	"encoding/json"
	"os"
	"testing"
)

type writerTestMockConfiguration struct {
	InterfaceName    string `json:"InterfaceName"`
	InterfaceIPCIDR  string `json:"InterfaceIPCIDR"`
	InterfaceAddress string `json:"InterfaceAddress"`
}

func TestWriteSuccess(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.json"
	w := newWriter(filePath)

	expected := writerTestMockConfiguration{
		InterfaceName:    "writerInterface0",
		InterfaceIPCIDR:  "10.0.0.0/24",
		InterfaceAddress: "10.0.0.1",
	}
	if err := w.Write(expected); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	content, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatalf("Failed to read file: %v", readErr)
	}

	var actual writerTestMockConfiguration
	if unmarshalErr := json.Unmarshal(content, &actual); unmarshalErr != nil {
		t.Fatalf("Error unmarshalling JSON: %v", unmarshalErr)
	}

	if actual != expected {
		t.Errorf("Written configuration %+v does not match expected %+v", actual, expected)
	}
}

func TestWriteFileCreateError(t *testing.T) {
	w := newWriter("")
	conf := writerTestMockConfiguration{
		InterfaceName:    "writerInterface1",
		InterfaceIPCIDR:  "10.0.1.0/24",
		InterfaceAddress: "10.0.1.1",
	}
	if err := w.Write(conf); err == nil {
		t.Error("Expected error for invalid file path, got none")
	}
}
