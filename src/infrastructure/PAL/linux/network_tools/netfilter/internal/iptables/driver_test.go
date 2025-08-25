package iptables

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ---- PAL.Commander mock -----------------------------------------------------

// fakeCmd is a table-driven mock for PAL.Commander.
// It satisfies CombinedOutput, Output, and Run.
type fakeCmd struct {
	// key: "bin arg1 arg2 ..."
	M map[string]struct {
		out []byte
		err error
	}
}

func newFake() *fakeCmd {
	return &fakeCmd{M: map[string]struct {
		out []byte
		err error
	}{}}
}

func (f *fakeCmd) key(bin string, args ...string) string {
	return strings.TrimSpace(bin + " " + strings.Join(args, " "))
}

func (f *fakeCmd) set(bin string, args []string, out []byte, err error) {
	f.M[f.key(bin, args...)] = struct {
		out []byte
		err error
	}{out: out, err: err}
}

// --- PAL.Commander impl ---

func (f *fakeCmd) CombinedOutput(name string, args ...string) ([]byte, error) {
	k := f.key(name, args...)
	if r, ok := f.M[k]; ok {
		return r.out, r.err
	}
	return nil, fmt.Errorf("no mock for: %s", k)
}

func (f *fakeCmd) Output(name string, args ...string) ([]byte, error) {
	// For tests we route to the same table as CombinedOutput.
	return f.CombinedOutput(name, args...)
}

func (f *fakeCmd) Run(name string, args ...string) error {
	_, err := f.CombinedOutput(name, args...)
	return err
}

// ---- small spec builders -----------------------------------------------------

func masqSpec(dev string) []string {
	return []string{"-o", dev, "-j", "MASQUERADE"}
}

func fwdSpec(iif, oif string) []string {
	return []string{"-i", iif, "-o", oif, "-j", "ACCEPT"}
}

func estConntrackSpec(iif, oif string) []string {
	return []string{"-i", iif, "-o", oif, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
}

func estStateSpec(iif, oif string) []string {
	return []string{"-i", iif, "-o", oif, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
}

// ---- tests ------------------------------------------------------------------

func TestEnableDevMasquerade_V4V6_Success(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")

	// v4: not present -> add
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, masqSpec("eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "nat", "-A", "POSTROUTING"}, masqSpec("eth0")...), []byte(""), nil)

	// v6: not present -> add
	f.set("ip6tables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, masqSpec("eth0")...), nil, errors.New("nope"))
	f.set("ip6tables", append([]string{"-t", "nat", "-A", "POSTROUTING"}, masqSpec("eth0")...), []byte(""), nil)

	if err := d.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("EnableDevMasquerade err: %v", err)
	}
}

func TestEnableDevMasquerade_EmptyDev_Err(t *testing.T) {
	d := NewDriverWithBinaries(newFake(), "iptables", "ip6tables")
	if err := d.EnableDevMasquerade(""); err == nil {
		t.Fatal("want error on empty dev, got nil")
	}
}

func TestEnableDevMasquerade_RuleExists_NoAdd(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, masqSpec("eth0")...), []byte("ok"), nil)
	if err := d.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestDisableDevMasquerade_V4_V6(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")

	// v4 present -> delete
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, masqSpec("eth0")...), []byte("ok"), nil)
	f.set("iptables", append([]string{"-t", "nat", "-D", "POSTROUTING"}, masqSpec("eth0")...), []byte(""), nil)

	// v6 present -> delete
	f.set("ip6tables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, masqSpec("eth0")...), []byte("ok"), nil)
	f.set("ip6tables", append([]string{"-t", "nat", "-D", "POSTROUTING"}, masqSpec("eth0")...), []byte(""), nil)

	if err := d.DisableDevMasquerade("eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestDisableDevMasquerade_NotPresent_NoDelete(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// not present -> -C errors => no -D expected
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, masqSpec("eth0")...), nil, errors.New("nope"))

	if err := d.DisableDevMasquerade("eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEnableForwardingFromTunToDev_DockerUser_FallbackConntrackToState(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	// DOCKER-USER exists
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, []byte("chain"), nil)

	// tun->dev: not exist -> add
	f.set("iptables", append([]string{"-t", "filter", "-C", "DOCKER-USER"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "DOCKER-USER"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)

	// dev->tun: conntrack add fails -> fallback to state
	f.set("iptables", append([]string{"-t", "filter", "-C", "DOCKER-USER"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "DOCKER-USER"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("boom"))

	f.set("iptables", append([]string{"-t", "filter", "-C", "DOCKER-USER"}, estStateSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "DOCKER-USER"}, estStateSpec("eth0", "tun0")...), []byte(""), nil)

	if err := d.EnableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEnableForwardingFromTunToDev_Forward_NoDockerUser(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	// DOCKER-USER missing -> FORWARD
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))

	// tun->dev: not exist -> add
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)

	// dev->tun: conntrack OK (no fallback)
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte(""), nil)

	if err := d.EnableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestDisableForwardingFromTunToDev_DeletePaths(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	// Use FORWARD
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))

	// direct rule present -> delete ok
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte("ok"), nil)
	f.set("iptables", append([]string{"-t", "filter", "-D", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)

	// established: try conntrack delete, force error to trigger fallback to state
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte("ok"), nil)
	f.set("iptables", append([]string{"-t", "filter", "-D", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("boom"))
	// fallback state delete: present -> delete ok
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estStateSpec("eth0", "tun0")...), []byte("ok"), nil)
	f.set("iptables", append([]string{"-t", "filter", "-D", "FORWARD"}, estStateSpec("eth0", "tun0")...), []byte(""), nil)

	if err := d.DisableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEnableForwardingFromDevToTun_Delegates(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	// No DOCKER-USER -> FORWARD
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte(""), nil)

	if err := d.EnableForwardingFromDevToTun("tun0", "eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestConfigureMssClamping_V4Only(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	// FORWARD
	f.set("iptables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)

	// OUTPUT
	f.set("iptables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)

	if err := d.ConfigureMssClamping(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestConfigureMssClamping_V6Also(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")

	// v4
	f.set("iptables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	f.set("iptables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)

	// v6
	f.set("ip6tables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("ip6tables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	f.set("ip6tables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("ip6tables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)

	if err := d.ConfigureMssClamping(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestExec_ErrorIncludesTrimmedOutput(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	spec := fwdSpec("tun0", "eth0")
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, spec...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, spec...), []byte(" some stderr text \n"), errors.New("boom"))

	err := d.addIfMissing("iptables", "filter", "FORWARD", spec...)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "out: "+string(bytes.TrimSpace([]byte(" some stderr text \n")))) {
		t.Fatalf("error should include trimmed output, got: %q", err.Error())
	}
}

func TestDelIfPresent_NoRule_NoDelete(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")

	spec := masqSpec("eth0")
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, spec...), nil, errors.New("nope"))

	if err := d.delIfPresent("iptables", "nat", "POSTROUTING", spec...); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
func TestNewDriverWithBinaries_Defaults(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "", "")
	if d.ipt4 != "iptables" {
		t.Fatalf("ipt4 default must be iptables, got %q", d.ipt4)
	}
	if d.ipt6 != "" {
		t.Fatalf("ipt6 should stay empty when not provided")
	}
}

func TestDisableDevMasquerade_EmptyDev_Err(t *testing.T) {
	d := NewDriverWithBinaries(newFake(), "iptables", "")
	if err := d.DisableDevMasquerade(""); err == nil {
		t.Fatal("want error on empty dev, got nil")
	}
}

func TestEnableForwarding_InputEmpty_Err(t *testing.T) {
	d := NewDriverWithBinaries(newFake(), "iptables", "")
	if err := d.EnableForwardingFromTunToDev("", "eth0"); err == nil {
		t.Fatal("want error on empty iface")
	}
	if err := d.EnableForwardingFromTunToDev("tun0", ""); err == nil {
		t.Fatal("want error on empty iface")
	}
}

func TestDisableForwarding_InputEmpty_Err(t *testing.T) {
	d := NewDriverWithBinaries(newFake(), "iptables", "")
	if err := d.DisableForwardingFromTunToDev("", "eth0"); err == nil {
		t.Fatal("want error on empty iface")
	}
	if err := d.DisableForwardingFromTunToDev("tun0", ""); err == nil {
		t.Fatal("want error on empty iface")
	}
}

func TestEnableDevMasquerade_V4_AddError_Annotated(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// rule not present -> add -> add returns error
	spec := masqSpec("eth0")
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, spec...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "nat", "-A", "POSTROUTING"}, spec...), nil, errors.New("boom"))
	err := d.EnableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "enable v4 masquerade on eth0") {
		t.Fatalf("expect annotated v4 error, got %v", err)
	}
}

func TestEnableDevMasquerade_V6_AddError_Annotated(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")
	// v4 passes
	spec := masqSpec("eth0")
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, spec...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "nat", "-A", "POSTROUTING"}, spec...), []byte(""), nil)
	// v6 add fails
	f.set("ip6tables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, spec...), nil, errors.New("nope"))
	f.set("ip6tables", append([]string{"-t", "nat", "-A", "POSTROUTING"}, spec...), nil, errors.New("boom"))
	err := d.EnableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "enable v6 masquerade on eth0") {
		t.Fatalf("expect annotated v6 error, got %v", err)
	}
}

func TestEnableForwarding_V4Forward_AddError(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// choose FORWARD (no DOCKER-USER)
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	// forward rule add fails
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("boom"))
	err := d.EnableForwardingFromTunToDev("tun0", "eth0")
	if err == nil || !strings.Contains(err.Error(), "v4 forward tun0->eth0") {
		t.Fatalf("expect annotated v4 forward error, got %v", err)
	}
}

func TestEnableForwarding_V4Reverse_AddError(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// FORWARD chain
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	// direct rule OK
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)
	// conntrack add fails
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("boom"))
	// state add fails too -> bubble
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estStateSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, estStateSpec("eth0", "tun0")...), nil, errors.New("boom2"))
	err := d.EnableForwardingFromTunToDev("tun0", "eth0")
	if err == nil || !strings.Contains(err.Error(), "v4 reverse forward eth0->tun0") {
		t.Fatalf("expect annotated v4 reverse error, got %v", err)
	}
}

func TestEnableForwarding_V6ForwardAndReverse_AddErrors(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")
	// v4 passes
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte(""), nil)

	// v6 forward error
	f.set("ip6tables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	f.set("ip6tables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("ip6tables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("boom"))
	if err := d.EnableForwardingFromTunToDev("tun0", "eth0"); err == nil || !strings.Contains(err.Error(), "v6 forward tun0->eth0") {
		t.Fatalf("expect annotated v6 forward error, got %v", err)
	}

	// v6 reverse error (separate run)
	f2 := newFake()
	d2 := NewDriverWithBinaries(f2, "iptables", "ip6tables")
	// v4 passes again
	f2.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	f2.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f2.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte(""), nil)
	f2.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f2.set("iptables", append([]string{"-t", "filter", "-A", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte(""), nil)
	// v6 reverse: both conntrack and state fail
	f2.set("ip6tables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	f2.set("ip6tables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), []byte("ok"), nil)
	f2.set("ip6tables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f2.set("ip6tables", append([]string{"-t", "filter", "-A", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("boom"))
	f2.set("ip6tables", append([]string{"-t", "filter", "-C", "FORWARD"}, estStateSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f2.set("ip6tables", append([]string{"-t", "filter", "-A", "FORWARD"}, estStateSpec("eth0", "tun0")...), nil, errors.New("boom2"))
	if err := d2.EnableForwardingFromTunToDev("tun0", "eth0"); err == nil || !strings.Contains(err.Error(), "v6 reverse forward eth0->tun0") {
		t.Fatalf("expect annotated v6 reverse error, got %v", err)
	}
}

func TestDelEstablishedRuleIfPresent_SuccessConntrack_NoFallback(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// rule present and delete ok => no fallback
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte("ok"), nil)
	f.set("iptables", append([]string{"-t", "filter", "-D", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), []byte(""), nil)
	if err := d.delEstablishedRuleIfPresent("iptables", "filter", "FORWARD", "eth0", "tun0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestDelEstablishedRuleIfPresent_NothingPresent_Noop(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// conntrack -C errors (not present); state -C errors (not present) -> noop
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estStateSpec("eth0", "tun0")...), nil, errors.New("nope"))
	if err := d.delEstablishedRuleIfPresent("iptables", "filter", "FORWARD", "eth0", "tun0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestDelIfPresent_DeleteError_Bubbled(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	spec := masqSpec("eth0")
	// present -> -D fails with error
	f.set("iptables", append([]string{"-t", "nat", "-C", "POSTROUTING"}, spec...), []byte("ok"), nil)
	f.set("iptables", append([]string{"-t", "nat", "-D", "POSTROUTING"}, spec...), nil, errors.New("boom"))
	err := d.delIfPresent("iptables", "nat", "POSTROUTING", spec...)
	if err == nil || !strings.Contains(err.Error(), "iptables -t nat -D POSTROUTING") {
		t.Fatalf("expected formatted delete error, got %v", err)
	}
}

func TestDisableForwardingFromDevToTun_Delegates(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// choose FORWARD
	f.set("iptables", []string{"-t", "filter", "-nL", "DOCKER-USER"}, nil, errors.New("no chain"))
	// not present -> nothing to delete
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, fwdSpec("tun0", "eth0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estConntrackSpec("eth0", "tun0")...), nil, errors.New("nope"))
	f.set("iptables", append([]string{"-t", "filter", "-C", "FORWARD"}, estStateSpec("eth0", "tun0")...), nil, errors.New("nope"))
	if err := d.DisableForwardingFromDevToTun("tun0", "eth0"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

/* ---- ConfigureMssClamping error branches (hit all four lines) ---- */

func TestConfigureMssClamping_V4Forward_Error(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// v4 FORWARD fails
	f.set("iptables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("boom"))
	err := d.ConfigureMssClamping()
	if err == nil || !strings.Contains(err.Error(), "v4 MSS clamp FORWARD") {
		t.Fatalf("expect v4 FORWARD error, got %v", err)
	}
}

func TestConfigureMssClamping_V4Output_Error(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "")
	// v4 FORWARD ok
	f.set("iptables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	// v4 OUTPUT fails
	f.set("iptables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("boom"))
	err := d.ConfigureMssClamping()
	if err == nil || !strings.Contains(err.Error(), "v4 MSS clamp OUTPUT") {
		t.Fatalf("expect v4 OUTPUT error, got %v", err)
	}
}

func TestConfigureMssClamping_V6Forward_Error(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")
	// v4 ok
	f.set("iptables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	f.set("iptables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	// v6 FORWARD fails
	f.set("ip6tables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("ip6tables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("boom"))
	err := d.ConfigureMssClamping()
	if err == nil || !strings.Contains(err.Error(), "v6 MSS clamp FORWARD") {
		t.Fatalf("expect v6 FORWARD error, got %v", err)
	}
}

func TestConfigureMssClamping_V6Output_Error(t *testing.T) {
	f := newFake()
	d := NewDriverWithBinaries(f, "iptables", "ip6tables")
	// v4 ok
	f.set("iptables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	f.set("iptables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("iptables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	// v6 FORWARD ok, v6 OUTPUT fails
	f.set("ip6tables", []string{"-t", "mangle", "-C", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("ip6tables", []string{"-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, []byte(""), nil)
	f.set("ip6tables", []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("nope"))
	f.set("ip6tables", []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}, nil, errors.New("boom"))
	err := d.ConfigureMssClamping()
	if err == nil || !strings.Contains(err.Error(), "v6 MSS clamp OUTPUT") {
		t.Fatalf("expect v6 OUTPUT error, got %v", err)
	}
}
