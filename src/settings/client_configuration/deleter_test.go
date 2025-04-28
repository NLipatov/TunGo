package client_configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDeleter_Delete_Success(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.conf")
	if err := os.WriteFile(fpath, []byte("data"), 0o600); err != nil {
		t.Fatalf("setup: write file: %v", err)
	}

	d := NewDefaultDeleter(nil)
	if err := d.Delete(fpath); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if _, err := os.Stat(fpath); !os.IsNotExist(err) {
		t.Errorf("file still exists after Delete, stat error = %v", err)
	}
}

func TestDefaultDeleter_Delete_NotExist(t *testing.T) {
	non := filepath.Join(t.TempDir(), "no-such-file")
	d := NewDefaultDeleter(nil)
	err := d.Delete(non)
	if err == nil {
		t.Fatal("Delete() error = nil, want non-nil")
	}
}
