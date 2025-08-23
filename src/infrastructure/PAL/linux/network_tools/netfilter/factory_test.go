package netfilter

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"tungo/application"
)

// ---------- test doubles ----------

type stubNetfilter struct{}

func (stubNetfilter) EnableDevMasquerade(string) error                   { return nil }
func (stubNetfilter) DisableDevMasquerade(string) error                  { return nil }
func (stubNetfilter) EnableForwardingFromTunToDev(string, string) error  { return nil }
func (stubNetfilter) DisableForwardingFromTunToDev(string, string) error { return nil }
func (stubNetfilter) EnableForwardingFromDevToTun(string, string) error  { return nil }
func (stubNetfilter) DisableForwardingFromDevToTun(string, string) error { return nil }
func (stubNetfilter) ConfigureMssClamping() error                        { return nil }

// Commander mock
type cmdResp struct {
	out []byte
	err error
}
type mockCmd struct {
	calls []string
	m     map[string]cmdResp
}

func newMockCmd() *mockCmd { return &mockCmd{m: map[string]cmdResp{}} }

func key(name string, args ...string) string { return name + "\x00" + strings.Join(args, "\x00") }

func (c *mockCmd) set(name string, args []string, out []byte, err error) {
	c.m[key(name, args...)] = cmdResp{out: out, err: err}
}

func (c *mockCmd) CombinedOutput(name string, args ...string) ([]byte, error) {
	k := key(name, args...)
	c.calls = append(c.calls, k)
	if r, ok := c.m[k]; ok {
		return r.out, r.err
	}
	return nil, fmt.Errorf("unexpected call: %s %v", name, args)
}
func (c *mockCmd) Output(name string, args ...string) ([]byte, error) {
	return c.CombinedOutput(name, args...)
}
func (c *mockCmd) Run(name string, args ...string) error {
	_, err := c.CombinedOutput(name, args...)
	return err
}

// nft probe / factories mocks
type mockProbe struct {
	ok  bool
	err error
}

func (p mockProbe) Supports() (bool, error) { return p.ok, p.err }

type mockNFTFactory struct {
	ret application.Netfilter
	err error
	n   int
}

func (f *mockNFTFactory) New() (application.Netfilter, error) {
	f.n++
	return f.ret, f.err
}

type mockIPTFactory struct {
	gotV4 string
	gotV6 string
	ret   application.Netfilter
	n     int
}

func (f *mockIPTFactory) New(v4bin, v6bin string) application.Netfilter {
	f.n++
	f.gotV4, f.gotV6 = v4bin, v6bin
	return f.ret
}

// ---------- tests for Build ----------

func TestBuild_PrefersNFT_WhenProbeOK(t *testing.T) {
	cmd := newMockCmd() // should not be used
	f := NewFactory(cmd).
		WithProbe(mockProbe{ok: true}).
		WithNFTFactory(&mockNFTFactory{ret: stubNetfilter{}})

	nf, err := f.Build()
	if err != nil {
		t.Fatalf("Build() err = %v", err)
	}
	if _, ok := nf.(stubNetfilter); !ok {
		t.Fatalf("expected nft backend, got %T", nf)
	}
	if len(cmd.calls) != 0 {
		t.Fatalf("did not expect any command calls, got %v", cmd.calls)
	}
}

func TestBuild_NFTProbeOKButFactoryFails_FallsBackToIptablesLegacy(t *testing.T) {
	cmd := newMockCmd()
	// Report iptables-legacy and ip6tables-legacy as available.
	cmd.set("iptables-legacy", []string{"-V"}, []byte("iptables v1.8.11 (legacy)"), nil)
	cmd.set("ip6tables-legacy", []string{"-V"}, []byte("ip6tables v1.8.11 (legacy)"), nil)

	nftF := &mockNFTFactory{err: errors.New("boom")}
	iptF := &mockIPTFactory{ret: stubNetfilter{}}

	f := NewFactory(cmd).
		WithProbe(mockProbe{ok: true}).
		WithNFTFactory(nftF).
		WithIPTablesFactory(iptF)

	nf, err := f.Build()
	if err != nil {
		t.Fatalf("Build() err = %v", err)
	}
	if _, ok := nf.(stubNetfilter); !ok {
		t.Fatalf("expected iptables backend, got %T", nf)
	}
	if iptF.gotV4 != "iptables-legacy" || iptF.gotV6 != "ip6tables-legacy" {
		t.Fatalf("expected legacy bins, got v4=%q v6=%q", iptF.gotV4, iptF.gotV6)
	}
}

func TestBuild_NoNFT_NoLegacy_UsesIptablesLegacyMode(t *testing.T) {
	cmd := newMockCmd()
	// No iptables-legacy:
	// iptables (legacy) present:
	cmd.set("iptables", []string{"-V"}, []byte("iptables v1.8.11 (legacy)"), nil)
	cmd.set("ip6tables", []string{"-V"}, []byte("ip6tables v1.8.11 (legacy)"), nil)

	iptF := &mockIPTFactory{ret: stubNetfilter{}}

	f := NewFactory(cmd).
		WithProbe(mockProbe{ok: false}).
		WithIPTablesFactory(iptF)

	nf, err := f.Build()
	if err != nil {
		t.Fatalf("Build() err = %v", err)
	}
	if _, ok := nf.(stubNetfilter); !ok {
		t.Fatalf("expected iptables(legacy) backend, got %T", nf)
	}
	if iptF.gotV4 != "iptables" || iptF.gotV6 != "ip6tables" {
		t.Fatalf("expected bins iptables/ip6tables, got v4=%q v6=%q", iptF.gotV4, iptF.gotV6)
	}
}

func TestBuild_NoNFT_NoLegacy_IptablesNFTMode_ReturnsError(t *testing.T) {
	cmd := newMockCmd()
	// iptables prints (nf_tables)
	cmd.set("iptables", []string{"-V"}, []byte("iptables v1.8.11 (nf_tables)"), nil)

	f := NewFactory(cmd).WithProbe(mockProbe{ok: false})

	_, err := f.Build()
	if err == nil || !strings.Contains(err.Error(), "iptables is (nf_tables) but nftables is unavailable") {
		t.Fatalf("expected nf_tables error, got %v", err)
	}
}

func TestBuild_NoNFT_NoLegacy_IptablesVErrorsWithProtocolNotSupported(t *testing.T) {
	cmd := newMockCmd()
	// iptables -V fails with typical Alpine message when kernel lacks nf_tables
	out := []byte("Failed to initialize nft: Protocol not supported")
	cmd.set("iptables", []string{"-V"}, out, errors.New("Failed to initialize nft: Protocol not supported"))

	f := NewFactory(cmd).WithProbe(mockProbe{ok: false})

	_, err := f.Build()
	if err == nil || !strings.Contains(err.Error(), "iptables uses nft shim") {
		t.Fatalf("expected nft shim error, got %v", err)
	}
}

func TestBuild_NoBackends_ReturnsFinalError(t *testing.T) {
	cmd := newMockCmd()
	// iptables-legacy missing; iptables -V returns something without markers
	cmd.set("iptables", []string{"-V"}, []byte("iptables v1.8.11"), nil)

	f := NewFactory(cmd).WithProbe(mockProbe{ok: false})

	_, err := f.Build()
	if err == nil || !strings.Contains(err.Error(), "no netfilter backend available") {
		t.Fatalf("expected final no-backend error, got %v", err)
	}
}

// IPv6 companion optional: present for v4 legacy but missing ip6tables-legacy
func TestBuild_LegacyV4_NoIPv6Companion_OK(t *testing.T) {
	cmd := newMockCmd()
	// v4 legacy is available
	cmd.set("iptables-legacy", []string{"-V"}, []byte("iptables v1.8.11 (legacy)"), nil)

	iptF := &mockIPTFactory{ret: stubNetfilter{}}
	f := NewFactory(cmd).
		WithProbe(mockProbe{ok: false}).
		WithIPTablesFactory(iptF)

	nf, err := f.Build()
	if err != nil {
		t.Fatalf("Build() err = %v", err)
	}
	if _, ok := nf.(stubNetfilter); !ok {
		t.Fatalf("expected iptables backend, got %T", nf)
	}
	if iptF.gotV4 != "iptables-legacy" || iptF.gotV6 != "" {
		t.Fatalf("expected v4=iptables-legacy and empty v6, got v4=%q v6=%q", iptF.gotV4, iptF.gotV6)
	}
}

// ---------- tests for helpers ----------

func Test_hasBinaryWorks_TrimsSpaces(t *testing.T) {
	cmd := newMockCmd()
	cmd.set("foo", []string{"-V"}, []byte("   \n   "), nil) // should be treated as not OK
	f := NewFactory(cmd)

	if _, ok := f.hasBinaryWorks("foo"); ok {
		t.Fatalf("expected hasBinaryWorks=false on whitespace-only output")
	}
}

func Test_detectIP6Companion(t *testing.T) {
	cmd := newMockCmd()
	cmd.set("ip6tables", []string{"-V"}, []byte("ip6tables v1.8.11 (legacy)"), nil)

	f := NewFactory(cmd)
	if v6, ok := f.detectIP6Companion("iptables"); !ok || v6 != "ip6tables" {
		t.Fatalf("expected ip6tables companion, got %q ok=%v", v6, ok)
	}

	if _, ok := f.detectIP6Companion("weird"); ok {
		t.Fatalf("expected no companion for unknown v4bin")
	}
}

func Test_iptablesMode(t *testing.T) {
	cmd := newMockCmd()
	cmd.set("iptables", []string{"-V"}, []byte("iptables v1.8.11 (legacy)"), nil)
	f := NewFactory(cmd)

	mode, out, err := f.iptablesMode("iptables")
	if err != nil || mode != "legacy" || !bytes.Contains(out, []byte("legacy")) {
		t.Fatalf("expected legacy mode, got mode=%q err=%v out=%q", mode, err, string(out))
	}

	// switch to nf_tables
	cmd2 := newMockCmd()
	cmd2.set("iptables", []string{"-V"}, []byte("iptables v1.8.11 (nf_tables)"), nil)
	f2 := NewFactory(cmd2)
	mode, out, err = f2.iptablesMode("iptables")
	if err != nil || mode != "nf_tables" || !bytes.Contains(out, []byte("nf_tables")) {
		t.Fatalf("expected nf_tables mode, got mode=%q err=%v out=%q", mode, err, string(out))
	}

	// unknown
	cmd3 := newMockCmd()
	cmd3.set("iptables", []string{"-V"}, []byte("iptables vX.Y.Z"), nil)
	f3 := NewFactory(cmd3)
	mode, _, err = f3.iptablesMode("iptables")
	if err != nil || mode != "" {
		t.Fatalf("expected empty mode, got mode=%q err=%v", mode, err)
	}
}

func Test_looksLikeNFTButKernelLacksSupport(t *testing.T) {
	cmd := newMockCmd()
	f := NewFactory(cmd)

	err := errors.New("Failed to initialize nft: Protocol not supported")
	out := []byte("Failed to initialize nft: Protocol not supported")
	if !f.looksLikeNFTButKernelLacksSupport(err, out) {
		t.Fatalf("expected detection of nft shim without kernel support")
	}
}
