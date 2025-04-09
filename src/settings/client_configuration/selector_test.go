package client_configuration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selectorTestResolver implements the Resolver interface for testing purposes.
type selectorTestResolver struct {
	// resolvePath is the path to be returned by Resolve.
	resolvePath string
	// err is the error to be returned by Resolve.
	err error
}

// Resolve returns the preset path and error.
func (f *selectorTestResolver) Resolve() (string, error) {
	return f.resolvePath, f.err
}

// TestSelectorStatError checks that Select returns an error when the configuration file does not exist.
func TestSelectorStatError(t *testing.T) {
	nonExistingFile := "/non/existent/config.yaml"
	resolver := &selectorTestResolver{resolvePath: "/dummy/dest.yaml"}
	selector := NewDefaultSelector(resolver)

	err := selector.Select(nonExistingFile)
	if err == nil || !strings.Contains(err.Error(), nonExistingFile) {
		t.Fatalf("expected error mentioning file %q, got %v", nonExistingFile, err)
	}
}

// TestSelectorReadFileError simulates an error during reading the configuration file by removing read permission.
func TestSelectorReadFileError(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "config.yaml")
	// Create a configuration file.
	if err := os.WriteFile(filePath, []byte("config data"), 0600); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Remove read permission to simulate a read error.
	if err := os.Chmod(filePath, 0200); err != nil {
		t.Fatalf("failed to change file permissions: %v", err)
	}
	resolver := &selectorTestResolver{resolvePath: filepath.Join(tempDir, "dest.yaml")}
	selector := NewDefaultSelector(resolver)

	err := selector.Select(filePath)
	if err == nil {
		t.Fatal("expected read file error, got nil")
	}

	// Restore permissions so that t.TempDir cleanup doesn't fail.
	if err := os.Chmod(filePath, 0600); err != nil {
		t.Fatalf("failed to restore file permissions: %v", err)
	}
}

// TestSelectorResolverError checks that Select returns an error if the resolver fails.
func TestSelectorResolverError(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "config.yaml")
	// Create a valid configuration file.
	if err := os.WriteFile(filePath, []byte("config data"), 0600); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	expectedErr := errors.New("resolver error")
	resolver := &selectorTestResolver{err: expectedErr}
	selector := NewDefaultSelector(resolver)

	err := selector.Select(filePath)
	if err == nil || err.Error() != expectedErr.Error() {
		t.Fatalf("expected resolver error %q, got %v", expectedErr, err)
	}
}

// TestSelectorWriteFileError simulates a write error by setting the destination as a directory.
func TestSelectorWriteFileError(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "config.yaml")
	configData := "configuration data"
	// Create a valid configuration file.
	if err := os.WriteFile(filePath, []byte(configData), 0600); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Use the temporary directory as the destination, which will cause a write error
	// because os.WriteFile expects a file path, not a directory.
	resolver := &selectorTestResolver{resolvePath: tempDir}
	selector := NewDefaultSelector(resolver)

	err := selector.Select(filePath)
	if err == nil {
		t.Fatal("expected write file error, got nil")
	}
}

// TestSelectorSuccess verifies that Select works correctly when all operations succeed.
func TestSelectorSuccess(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "config.yaml")
	configData := "successful configuration data"
	// Create a valid configuration file.
	if err := os.WriteFile(filePath, []byte(configData), 0600); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Define a valid destination path within the temporary directory.
	destPath := filepath.Join(tempDir, "dest.yaml")
	resolver := &selectorTestResolver{resolvePath: destPath}
	selector := NewDefaultSelector(resolver)

	if err := selector.Select(filePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the destination file exists and contains the expected data.
	writtenData, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(writtenData) != configData {
		t.Errorf("expected destination content %q, got %q", configData, string(writtenData))
	}
}
