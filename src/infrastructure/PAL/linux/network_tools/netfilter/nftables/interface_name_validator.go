package nftables

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
)

type InterfaceNameValidator interface {
	ValidateIfName(s string) error
}

const ifNameMaxLen = syscall.IFNAMSIZ - 1

type DefaultInterfaceNameValidator struct {
}

func NewDefaultInterfaceNameValidator() *DefaultInterfaceNameValidator {
	return &DefaultInterfaceNameValidator{}
}

func (d *DefaultInterfaceNameValidator) ValidateIfName(s string) error {
	if s == "" {
		return errors.New("iface name is empty")
	}
	if strings.ContainsRune(s, '/') {
		return fmt.Errorf("iface name contains '/': %q", s)
	}
	if strings.IndexByte(s, 0x00) >= 0 {
		return fmt.Errorf("iface name contains NUL byte: %q", s)
	}
	if len(s) > ifNameMaxLen {
		return fmt.Errorf("iface name too long (max %d): %q", ifNameMaxLen, s)
	}
	return nil
}
