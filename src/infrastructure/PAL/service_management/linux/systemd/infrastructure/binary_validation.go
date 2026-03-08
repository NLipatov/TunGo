package infrastructure

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

func ValidateTungoBinaryForSystemd(h Hooks, binaryPath string) error {
	info, err := h.Lstat(binaryPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("tungo executable is not installed at %s; install it using the official Linux guide", binaryPath)
		}
		return fmt.Errorf("failed to lstat %s: %w", binaryPath, err)
	}
	if info == nil {
		return fmt.Errorf("failed to lstat %s: empty file info", binaryPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink", binaryPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", binaryPath)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", binaryPath)
	}
	if info.Mode()&0o022 != 0 {
		return fmt.Errorf("%s must not be writable by group or others; run: sudo chmod 0755 %s", binaryPath, binaryPath)
	}
	uid, ok := fileOwnerUID(info)
	if !ok {
		return fmt.Errorf("failed to verify owner of %s", binaryPath)
	}
	if uid != 0 {
		return fmt.Errorf("%s must be owned by root; run: sudo chown root:root %s", binaryPath, binaryPath)
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint64, bool) {
	sys := info.Sys()
	if sys == nil {
		return 0, false
	}

	v := reflect.ValueOf(sys)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return 0, false
	}

	uidField := v.FieldByName("Uid")
	if !uidField.IsValid() {
		return 0, false
	}

	switch uidField.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uidField.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		uid := uidField.Int()
		if uid < 0 {
			return 0, false
		}
		return uint64(uid), true
	default:
		return 0, false
	}
}
