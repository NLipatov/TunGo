package client

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// observerTestResolver implements the Resolver interface for testing purposes.
type observerTestResolver struct {
	// resolvePath is the path to be returned by Resolve.
	resolvePath string
	// err is the error to be returned by Resolve.
	err error
}

// Resolve returns the preset path and error.
func (f *observerTestResolver) Resolve() (string, error) {
	return f.resolvePath, f.err
}

// TestObserverResolveError checks that Observe returns the error
// from the resolver if Resolve() fails.
func TestObserverResolveError(t *testing.T) {
	expectedErr := errors.New("resolve error")
	resolver := &observerTestResolver{err: expectedErr}
	observer := NewDefaultObserver(resolver)

	_, err := observer.Observe()
	if err == nil || err.Error() != expectedErr.Error() {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

// TestObserverGlobError checks the branch when filepath.Glob returns an error.
// For this purpose, we set an invalid pattern by using a file name with an unmatched bracket.
func TestObserverGlobError(t *testing.T) {
	// This path is used to form an invalid pattern.
	invalidPath := "/tmp/config["
	resolver := &observerTestResolver{resolvePath: invalidPath}
	observer := NewDefaultObserver(resolver)

	_, err := observer.Observe()
	if err == nil {
		t.Fatal("expected an error from filepath.Glob, but got none")
	}
}

// TestObserverReturnsMatches checks that DefaultObserver correctly filters out the default configuration file.
func TestObserverReturnsMatches(t *testing.T) {
	// Create a temporary directory.
	tempDir := t.TempDir()
	// Default configuration file name.
	defaultFilename := "config.yaml"
	defaultPath := filepath.Join(tempDir, defaultFilename)

	// Create the default configuration file.
	err := os.WriteFile(defaultPath, []byte("default configuration"), 0600)
	if err != nil {
		t.Fatalf("error creating default file: %v", err)
	}

	// Create additional files that match the glob pattern.
	extraFiles := []string{
		filepath.Join(tempDir, "config.yaml.bak"),
		filepath.Join(tempDir, "config.yaml.old"),
	}
	for _, file := range extraFiles {
		err = os.WriteFile(file, []byte("backup configuration"), 0600)
		if err != nil {
			t.Fatalf("error creating file %s: %v", file, err)
		}
	}

	// Use observerTestResolver to return the default configuration file path.
	resolver := &observerTestResolver{resolvePath: defaultPath}
	observer := NewDefaultObserver(resolver)

	results, err := observer.Observe()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create a map for quick lookup.
	matchesMap := make(map[string]bool)
	for _, res := range results {
		matchesMap[res] = true
	}

	expected := extraFiles
	expected = append(expected, defaultFilename)
	for _, expected := range extraFiles {
		if !matchesMap[expected] {
			t.Errorf("expected file %s not found in results", expected)
		}
	}
}
