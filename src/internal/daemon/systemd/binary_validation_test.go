package systemd

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type binaryMockFileInfo struct {
	mode os.FileMode
	sys  interface{}
}

func (m binaryMockFileInfo) Name() string       { return "tungo" }
func (m binaryMockFileInfo) Size() int64        { return 1 }
func (m binaryMockFileInfo) Mode() os.FileMode  { return m.mode }
func (m binaryMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m binaryMockFileInfo) IsDir() bool        { return m.mode.IsDir() }
func (m binaryMockFileInfo) Sys() interface{}   { return m.sys }

type statUID struct{ Uid uint32 }
type statUIDInt struct{ Uid int64 }
type statUIDString struct{ Uid string }
type statNoUID struct{ Gid uint32 }

type ptrUID struct{ Uid uint32 }

func TestValidateTungoBinaryForSystemd(t *testing.T) {
	binaryPath := "/usr/local/bin/tungo"
	baseHooks := Hooks{
		Lstat: func(string) (os.FileInfo, error) {
			return binaryMockFileInfo{mode: 0o755, sys: statUID{Uid: 0}}, nil
		},
	}

	t.Run("missing", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		customPath := "/opt/tungo/bin/tungo"
		err := ValidateTungoBinaryForSystemd(h, customPath)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), customPath) {
			t.Fatalf("expected error to mention custom binary path %q, got %q", customPath, err.Error())
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
			return binaryMockFileInfo{mode: os.ModeSymlink | 0o777, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not regular", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return binaryMockFileInfo{mode: os.ModeDir | 0o755, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not executable", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return binaryMockFileInfo{mode: 0o644, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("group writable", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return binaryMockFileInfo{mode: 0o775, sys: statUID{Uid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("owner unknown", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return binaryMockFileInfo{mode: 0o755, sys: statNoUID{Gid: 0}}, nil
		}
		if err := ValidateTungoBinaryForSystemd(h, binaryPath); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("owner non-root", func(t *testing.T) {
		h := baseHooks
		h.Lstat = func(string) (os.FileInfo, error) {
			return binaryMockFileInfo{mode: 0o755, sys: statUID{Uid: 1000}}, nil
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
	if _, ok := fileOwnerUID(binaryMockFileInfo{sys: nil}); ok {
		t.Fatal("expected false for nil sys")
	}

	var p *ptrUID
	if _, ok := fileOwnerUID(binaryMockFileInfo{sys: p}); ok {
		t.Fatal("expected false for nil pointer")
	}
	p = &ptrUID{Uid: 9}
	if uid, ok := fileOwnerUID(binaryMockFileInfo{sys: p}); !ok || uid != 9 {
		t.Fatalf("expected uid=9 from non-nil pointer, got %d ok=%v", uid, ok)
	}

	if _, ok := fileOwnerUID(binaryMockFileInfo{sys: 42}); ok {
		t.Fatal("expected false for non-struct sys")
	}

	if _, ok := fileOwnerUID(binaryMockFileInfo{sys: statNoUID{Gid: 1}}); ok {
		t.Fatal("expected false when Uid field missing")
	}

	if uid, ok := fileOwnerUID(binaryMockFileInfo{sys: statUID{Uid: 7}}); !ok || uid != 7 {
		t.Fatalf("expected uid=7, got %d ok=%v", uid, ok)
	}

	if uid, ok := fileOwnerUID(binaryMockFileInfo{sys: statUIDInt{Uid: 8}}); !ok || uid != 8 {
		t.Fatalf("expected uid=8, got %d ok=%v", uid, ok)
	}

	if _, ok := fileOwnerUID(binaryMockFileInfo{sys: statUIDInt{Uid: -1}}); ok {
		t.Fatal("expected false for negative int uid")
	}

	if _, ok := fileOwnerUID(binaryMockFileInfo{sys: statUIDString{Uid: "1"}}); ok {
		t.Fatal("expected false for unsupported uid kind")
	}
}
