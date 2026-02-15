package stat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultStat(t *testing.T) {
	s := NewDefaultStat()
	if s == nil {
		t.Fatal("expected non-nil stat implementation")
	}
	if _, ok := s.(*DefaultStat); !ok {
		t.Fatalf("expected *DefaultStat, got %T", s)
	}
}

func TestDefaultStat_Stat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	s := DefaultStat{}
	info, err := s.Stat(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name() != "file.txt" {
		t.Fatalf("unexpected filename: %q", info.Name())
	}
}

func TestDefaultStat_Stat_NotFound(t *testing.T) {
	s := DefaultStat{}
	_, err := s.Stat(filepath.Join(t.TempDir(), "missing.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}
