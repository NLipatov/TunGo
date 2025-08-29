package iptables

import (
	"errors"
	"fmt"
	"strings"
	"tungo/infrastructure/PAL"
)

type FamilyExec struct {
	v4bin, v6bin string
	cmd          PAL.Commander
	wait         WaitPolicy
	skip         Skipper
}

func NewFamilyExec(v4, v6 string, cmd PAL.Commander, wait WaitPolicy, skip Skipper) *FamilyExec {
	return &FamilyExec{v4bin: v4, v6bin: v6, cmd: cmd, wait: wait, skip: skip}
}

func (e *FamilyExec) ExecBothFamilies(base []string, table string, natV6BestEffort bool) error {
	var errs []error

	args4 := append(append([]string{}, e.wait.Args("IPv4")...), base...)
	if out, err := e.run(e.v4bin, args4...); err != nil {
		errs = append(errs, fmt.Errorf("[IPv4] %s %v failed: %w, output: %s", e.v4bin, args4, err, out))
	}

	args6 := append(append([]string{}, e.wait.Args("IPv6")...), base...)
	if out, err := e.run(e.v6bin, args6...); err != nil {
		if !(natV6BestEffort && table == "nat" && e.looksLikeNatUnsupported(out)) {
			errs = append(errs, fmt.Errorf("[IPv6] %s %v failed: %w, output: %s", e.v6bin, args6, err, out))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (e *FamilyExec) run(bin string, args ...string) ([]byte, error) {
	if bin == "" {
		return nil, nil
	}
	if e.skip.CanSkip(bin, args...) {
		return nil, nil
	}
	return e.cmd.CombinedOutput(bin, args...)
}

func (e *FamilyExec) looksLikeNatUnsupported(out []byte) bool {
	s := strings.ToLower(string(out))
	return strings.Contains(s, "table `nat`") ||
		strings.Contains(s, "can't initialize ip6tables table `nat`") ||
		strings.Contains(s, "can't initialize ip6tables table nat") ||
		strings.Contains(s, "no chain/target/match by that name")
}
