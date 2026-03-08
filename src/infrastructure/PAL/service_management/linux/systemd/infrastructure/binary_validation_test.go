package infrastructure

import (
	"errors"
	"os"
	"testing"
	"time"
)

type mockFileInfo struct {
	mode os.FileMode
	sys  interface{}
}

func (m mockFileInfo) Name() string       { return "tungo" }
func (m mockFileInfo) Size() int64        { return 1 }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.mode.IsDir() }
func (m mockFileInfo) Sys() interface{}   { return m.sys }

type statUID struct{ Uid uint32 }
type statUIDInt struct{ Uid int64 }
type statUIDString struct{ Uid string }
type statNoUID struct{ Gid uint32 }

type ptrUID struct{ Uid uint32 }

func TestValidateTungoBinaryForSystemd(t *testing.T) {
	binaryPath := "/usr/local/bin/tungo"
	baseHooks := Hooks{
		Lstat: func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: 0o755, sys: statUID{Uid: 0}}, nil
		},
	}

	t.Run("missing", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("lstat error", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) { return nil, errors.New("boom") }
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil info", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) { return nil, nil }
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: os.ModeSymlink | 0o777, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not regular", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: os.ModeDir | 0o755, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not executable", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: 0o644, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("group writable", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: 0o775, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("owner unknown", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: 0o755, sys: statNoUID{Gid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("owner non-root", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return mockFileInfo{mode: 0o755, sys: statUID{Uid: 1000}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("ok", func(t *testing.T) {
		if err := ValidateTungoBinaryForSystemd(baseHooks, binaryPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFileOwnerUID(t *testing.T) {
	if _, ok := fileOwnerUID(mockFileInfo{sys: nil}); ok {
		t.Fatal("expected false for nil sys")
	}

	var p *ptrUID
	if _, ok := fileOwnerUID(mockFileInfo{sys: p}); ok {
		t.Fatal("expected false for nil pointer")
	}
	p = &ptrUID{Uid: 9}
	if uid, ok := fileOwnerUID(mockFileInfo{sys: p}); !ok || uid != 9 {
		t.Fatalf("expected uid=9 from non-nil pointer, got %d ok=%v", uid, ok)
	}

	if _, ok := fileOwnerUID(mockFileInfo{sys: 42}); ok {
		t.Fatal("expected false for non-struct sys")
	}

	if _, ok := fileOwnerUID(mockFileInfo{sys: statNoUID{Gid: 1}}); ok {
		t.Fatal("expected false when Uid field missing")
	}

	if uid, ok := fileOwnerUID(mockFileInfo{sys: statUID{Uid: 7}}); !ok || uid != 7 {
		t.Fatalf("expected uid=7, got %d ok=%v", uid, ok)
	}

	if uid, ok := fileOwnerUID(mockFileInfo{sys: statUIDInt{Uid: 8}}); !ok || uid != 8 {
		t.Fatalf("expected uid=8, got %d ok=%v", uid, ok)
	}

	if _, ok := fileOwnerUID(mockFileInfo{sys: statUIDInt{Uid: -1}}); ok {
		t.Fatal("expected false for negative int uid")
	}

	if _, ok := fileOwnerUID(mockFileInfo{sys: statUIDString{Uid: "1"}}); ok {
		t.Fatal("expected false for unsupported uid kind")
	}
}
