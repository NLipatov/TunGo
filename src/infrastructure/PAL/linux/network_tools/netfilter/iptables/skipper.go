package iptables

import (
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

type Skipper interface {
	CanSkip(bin string, args ...string) bool
}

type DefaultSkipper struct {
	v6bin string
	wait  WaitPolicy
	cmd   PAL.Commander
}

func NewSkipper(v6bin string, wait WaitPolicy, cmd PAL.Commander) *DefaultSkipper {
	return &DefaultSkipper{v6bin: v6bin, wait: wait, cmd: cmd}
}

func (s *DefaultSkipper) CanSkip(bin string, args ...string) bool {
	var table, action, chain string
	var rule []string

	i := 0
	for i < len(args) {
		switch a := args[i]; a {
		case "--wait":
			i++
		case "--wait-interval":
			i += 2
		case "-t":
			i++
			if i < len(args) {
				table = args[i]
				i++
			}
		case "-A", "-D", "-I":
			if action == "" {
				action = a
				i++
				if i < len(args) {
					chain = args[i]
					i++
				}
				if action == "-I" && i < len(args) {
					if _, err := strconv.Atoi(args[i]); err == nil {
						i++
					}
				}
			} else {
				i++
			}
		default:
			rule = append(rule, a)
			i++
		}
	}
	if action == "" || chain == "" {
		return false
	}

	fam := "IPv4"
	l := strings.ToLower(bin)
	if strings.Contains(l, "ip6tables") || bin == s.v6bin {
		fam = "IPv6"
	}

	check := append([]string{}, s.wait.Args(fam)...)
	if table != "" {
		check = append(check, "-t", table)
	}
	check = append(check, "-C", chain)
	check = append(check, rule...)

	_, err := s.cmd.CombinedOutput(bin, check...)
	exists := err == nil

	if action == "-D" {
		return !exists // skip deletion if rule does not exist
	}
	return exists
}
