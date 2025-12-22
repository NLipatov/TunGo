//go:build darwin

package scutil

import (
	"fmt"
	"tungo/infrastructure/PAL/exec_commander"
)

type v6 struct {
}

func NewV6() Contract {
	return &v6{}
}

func (s *v6) AddScopedDNSResolver(
	ifName string,
	nameservers []string,
	domains []string,
) error {
	if ifName == "" {
		return fmt.Errorf("scutil.v6: empty interface name")
	}
	if len(nameservers) == 0 {
		return fmt.Errorf("scutil.v6: no nameservers provided")
	}

	builder := newDNSScriptBuilder(ifName, nameservers, domains)
	script := builder.BuildAdd()

	cmd := exec_commander.NewStdinCommander(script)
	return cmd.Run("scutil")
}

func (s *v6) RemoveScopedDNSResolver(ifName string) error {
	if ifName == "" {
		return nil
	}

	builder := newDNSScriptBuilder(ifName, nil, nil)
	script := builder.BuildRemove()

	cmd := exec_commander.NewStdinCommander(script)
	return cmd.Run("scutil")
}
