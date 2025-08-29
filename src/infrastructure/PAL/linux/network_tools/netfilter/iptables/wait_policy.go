package iptables

import (
	"strconv"
	"sync"
	"tungo/infrastructure/PAL"
)

type WaitPolicy interface {
	Args(family string) []string
}

type DefaultWaitPolicy struct {
	v4bin, v6bin string
	cmd          PAL.Commander
	once         sync.Once
	v4ok, v6ok   bool
	waitMs       int
}

func NewWaitPolicy(v4bin, v6bin string, cmd PAL.Commander) *DefaultWaitPolicy {
	return &DefaultWaitPolicy{v4bin: v4bin, v6bin: v6bin, cmd: cmd, waitMs: 200}
}

func NewWaitPolicyWithWaitMS(v4bin, v6bin string, cmd PAL.Commander, waitMs int) *DefaultWaitPolicy {
	return &DefaultWaitPolicy{v4bin: v4bin, v6bin: v6bin, cmd: cmd, waitMs: waitMs}
}

func (w *DefaultWaitPolicy) Detect() {
	w.once.Do(func() {
		detect := func(bin string) bool {
			if bin == "" {
				return false
			}
			_, err := w.cmd.CombinedOutput(bin, "--wait", "--wait-interval", "1", "-S")
			return err == nil
		}
		w.v4ok = detect(w.v4bin)
		w.v6ok = detect(w.v6bin)
	})
}

func (w *DefaultWaitPolicy) SetInterval(ms int) {
	if ms > 0 {
		w.waitMs = ms
	}
}

func (w *DefaultWaitPolicy) Args(family string) []string {
	w.Detect()
	ok := w.v4ok
	if family == "IPv6" {
		ok = w.v6ok
	}
	if ok {
		return []string{"--wait", "--wait-interval", strconv.Itoa(w.waitMs)}
	}
	return []string{"--wait"}
}
