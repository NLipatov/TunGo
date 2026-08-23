package systemd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type binaryTestFileInfo struct {
	mode os.FileMode
	sys  any
}

func (i binaryTestFileInfo) Name() string       { return "tungo" }
func (i binaryTestFileInfo) Size() int64        { return 1 }
func (i binaryTestFileInfo) Mode() os.FileMode  { return i.mode }
func (i binaryTestFileInfo) ModTime() time.Time { return time.Time{} }
func (i binaryTestFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i binaryTestFileInfo) Sys() any           { return i.sys }

type binaryTestUID struct{ Uid uint32 }
type binaryTestIntUID struct{ Uid int64 }
type binaryTestStringUID struct{ Uid string }
type binaryTestNoUID struct{ Gid uint32 }

func TestValidateTungoBinaryForSystemd_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "tungo")
	err := validateTungoBinaryForSystemd(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("validateTungoBinaryForSystemd() error = %v", err)
	}
}

func TestValidateTungoBinaryInfo(t *testing.T) {
	path := "/usr/local/bin/tungo"
	tests := []struct {
		name string
		info os.FileInfo
		want string
	}{
		{
			name: "symlink",
			info: binaryTestFileInfo{mode: os.ModeSymlink | 0o777, sys: binaryTestUID{Uid: 0}},
			want: "must not be a symlink",
		},
		{
			name: "not regular",
			info: binaryTestFileInfo{mode: os.ModeDir | 0o755, sys: binaryTestUID{Uid: 0}},
			want: "not a regular file",
		},
		{
			name: "not executable",
			info: binaryTestFileInfo{mode: 0o644, sys: binaryTestUID{Uid: 0}},
			want: "not executable",
		},
		{
			name: "writable by group",
			info: binaryTestFileInfo{mode: 0o775, sys: binaryTestUID{Uid: 0}},
			want: "must not be writable",
		},
		{
			name: "owner unavailable",
			info: binaryTestFileInfo{mode: 0o755},
			want: "failed to verify owner",
		},
		{
			name: "non-root owner",
			info: binaryTestFileInfo{mode: 0o755, sys: binaryTestUID{Uid: 1000}},
			want: "must be owned by root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTungoBinaryInfo(tt.info, path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateTungoBinaryInfo() error = %v, want containing %q", err, tt.want)
			}
		})
	}

	info := binaryTestFileInfo{mode: 0o755, sys: binaryTestUID{Uid: 0}}
	if err := validateTungoBinaryInfo(info, path); err != nil {
		t.Fatalf("validateTungoBinaryInfo() error = %v", err)
	}
}

func TestFileOwnerUID(t *testing.T) {
	tests := []struct {
		name string
		sys  any
		uid  uint64
		ok   bool
	}{
		{name: "nil"},
		{name: "non-struct", sys: 42},
		{name: "missing field", sys: binaryTestNoUID{Gid: 1}},
		{name: "unsigned", sys: binaryTestUID{Uid: 7}, uid: 7, ok: true},
		{name: "signed", sys: binaryTestIntUID{Uid: 8}, uid: 8, ok: true},
		{name: "negative", sys: binaryTestIntUID{Uid: -1}},
		{name: "unsupported", sys: binaryTestStringUID{Uid: "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, ok := fileOwnerUID(binaryTestFileInfo{sys: tt.sys})
			if uid != tt.uid || ok != tt.ok {
				t.Fatalf("fileOwnerUID() = (%d, %v), want (%d, %v)", uid, ok, tt.uid, tt.ok)
			}
		})
	}

	var nilUID *binaryTestUID
	if _, ok := fileOwnerUID(binaryTestFileInfo{sys: nilUID}); ok {
		t.Fatal("fileOwnerUID() accepted nil pointer")
	}
}
