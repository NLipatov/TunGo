//go:build darwin

package scutil

import (
	"fmt"
	"tungo/infrastructure/PAL/exec_commander"
)

type v4 struct {
}

func NewV4() Contract {
	return &v4{}
}

func (s *v4) AddScopedDNSResolver(
	ifName string,
	nameservers []string,
	domains []string,
) error {
	if ifName == "" {
		return fmt.Errorf("scutil.v4: empty interface name")
	}
	if len(nameservers) == 0 {
		return fmt.Errorf("scutil.v4: no nameservers provided")
	}

	builder := newDNSScriptBuilder(ifName, nameservers, domains)
	script := builder.BuildAdd()

	cmd := exec_commander.NewStdinCommander(script)
	return cmd.Run("scutil")
}

func (s *v4) RemoveScopedDNSResolver(ifName string) error {
	if ifName == "" {
		return nil
	}

	builder := newDNSScriptBuilder(ifName, nil, nil)
	script := builder.BuildRemove()

	cmd := exec_commander.NewStdinCommander(script)
	return cmd.Run("scutil")
}
