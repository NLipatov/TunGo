//go:build darwin

package manager

import (
	"fmt"
	"net/netip"
	"testing"

	"tungo/internal/config/settings"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockIfconfig struct {
	linkAddrAddCalls []linkAddrAddCall
	setMTUCalls      []setMTUCall
	linkAddrAddErr   error
	setMTUErr        error
}

type linkAddrAddCall struct {
	ifName string
	cidr   string
}

type setMTUCall struct {
	ifName string
	mtu    int
}

func (m *mockIfconfig) LinkAddrAdd(ifName, cidr string) error {
	m.linkAddrAddCalls = append(m.linkAddrAddCalls, linkAddrAddCall{ifName, cidr})
	return m.linkAddrAddErr
}

func (m *mockIfconfig) SetMTU(ifName string, mtu int) error {
	m.setMTUCalls = append(m.setMTUCalls, setMTUCall{ifName, mtu})
	return m.setMTUErr
}

type mockRoute struct {
	getCalls       []string
	addCalls       []routeAddCall
	addViaGWCalls  []routeAddCall
	addSplitCalls  []string
	delSplitCalls  []string
	delCalls       []string
	defaultGWCalls int

	getErr       error
	addErr       error
	addViaGWErr  error
	addSplitErr  error
	delSplitErr  error
	delErr       error
	defaultGWVal string
	defaultGWErr error
}

type routeAddCall struct {
	ip    string
	iFace string
}

func (m *mockRoute) Get(destIP string) error {
	m.getCalls = append(m.getCalls, destIP)
	return m.getErr
}

func (m *mockRoute) Add(ip, iFace string) error {
	m.addCalls = append(m.addCalls, routeAddCall{ip, iFace})
	return m.addErr
}

func (m *mockRoute) AddViaGateway(ip, gw string) error {
	m.addViaGWCalls = append(m.addViaGWCalls, routeAddCall{ip, gw})
	return m.addViaGWErr
}

func (m *mockRoute) AddSplit(dev string) error {
	m.addSplitCalls = append(m.addSplitCalls, dev)
	return m.addSplitErr
}

func (m *mockRoute) DelSplit(dev string) error {
	m.delSplitCalls = append(m.delSplitCalls, dev)
	return m.delSplitErr
}

func (m *mockRoute) Del(destIP string) error {
	m.delCalls = append(m.delCalls, destIP)
	return m.delErr
}

func (m *mockRoute) DefaultGateway() (string, error) {
	m.defaultGWCalls++
	return m.defaultGWVal, m.defaultGWErr
}

type mockTunDevice struct {
	closed bool
}

func (m *mockTunDevice) Read(data []byte) (int, error)  { return 0, nil }
func (m *mockTunDevice) Write(data []byte) (int, error) { return 0, nil }
func (m *mockTunDevice) Close() error {
	m.closed = true
	return nil
}

type mockUTUN struct {
	closed bool
}

func (m *mockUTUN) Read([][]byte, []int, int) (int, error) { return 0, nil }
func (m *mockUTUN) Write([][]byte, int) (int, error)       { return 0, nil }
func (m *mockUTUN) Close() error {
	m.closed = true
	return nil
}
func (m *mockUTUN) Name() (string, error) { return "utun42", nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustIPHost(t *testing.T, raw string) settings.Host {
	t.Helper()
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return settings.Host{Domain: raw}
	}
	if ip.Unmap().Is4() {
		return settings.Host{IPv4: ip.Unmap().String()}
	}
	return settings.Host{IPv6: ip.String()}
}

func settingsV4Only(t *testing.T) settings.Settings {
	t.Helper()
	return settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "198.51.100.1"),
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
		},
		MTU: 1400,
	}
}

func settingsV6Only(t *testing.T) settings.Settings {
	t.Helper()
	return settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "2001:db8::1"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
		},
		MTU: 1400,
	}
}

func settingsDualStack(t *testing.T) settings.Settings {
	t.Helper()
	return settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "198.51.100.1"),
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
		},
		MTU: 1400,
	}
}

// ---------------------------------------------------------------------------
// Manager selection tests
// ---------------------------------------------------------------------------

func TestNew_V4Only(t *testing.T) {
	s := settingsV4Only(t)
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*v4); !ok {
		t.Fatalf("expected *v4, got %T", mgr)
	}
}

func TestNew_V6Only(t *testing.T) {
	s := settingsV6Only(t)
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*v6); !ok {
		t.Fatalf("expected *v6, got %T", mgr)
	}
}

func TestNew_DualStack(t *testing.T) {
	s := settingsDualStack(t)
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*dualStack); !ok {
		t.Fatalf("expected *dualStack, got %T", mgr)
	}
}

func TestNew_NoValidIP(t *testing.T) {
	s := settings.Settings{}
	_, err := New(s)
	if err == nil {
		t.Fatal("expected error for no valid IP, got nil")
	}
}

func TestNew_ZeroIPv4ReturnsV6Only(t *testing.T) {
	s := settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "2001:db8::1"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.Addr{}, // zero — not valid
			IPv6:       netip.MustParseAddr("fd00::2"),
		},
	}
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*v6); !ok {
		t.Fatalf("expected *v6, got %T", mgr)
	}
}

func TestNew_ZeroIPv6ReturnsV4Only(t *testing.T) {
	s := settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "198.51.100.1"),
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.Addr{}, // zero — not valid
		},
	}
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*v4); !ok {
		t.Fatalf("expected *v4, got %T", mgr)
	}
}

func TestNew_UnspecifiedIPv4NotTreatedAsValid(t *testing.T) {
	s := settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "2001:db8::1"),
			IPv4:       netip.MustParseAddr("0.0.0.0"), // unspecified
			IPv6:       netip.MustParseAddr("fd00::2"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
		},
	}
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*v6); !ok {
		t.Fatalf("expected *v6 (unspecified IPv4 is not valid), got %T", mgr)
	}
}

func TestNew_UnspecifiedIPv6NotTreatedAsValid(t *testing.T) {
	s := settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "198.51.100.1"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv6:       netip.MustParseAddr("::"),
		},
	}
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mgr.(*v4); !ok {
		t.Fatalf("expected *v4 (unspecified IPv6 is not valid), got %T", mgr)
	}
}

func TestNew_MappedIPv4AsIPv6TreatedAsV4(t *testing.T) {
	// ::ffff:10.0.0.2 is a v4-mapped-v6 address; Unmap().Is4() == true, so it is v4
	s := settings.Settings{
		Addressing: settings.Addressing{
			Server:     mustIPHost(t, "198.51.100.1"),
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("::ffff:192.168.1.1"),
		},
	}
	mgr, err := New(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The IPv6 is actually a mapped IPv4 so has6 is false; result should be v4-only.
	if _, ok := mgr.(*v4); !ok {
		t.Fatalf("expected *v4 (mapped v4 treated as v4), got %T", mgr)
	}
}

// ---------------------------------------------------------------------------
// v4 validateSettings
// ---------------------------------------------------------------------------

func TestV4_ValidateSettings_Valid(t *testing.T) {
	m := newV4(settingsV4Only(t), &mockIfconfig{}, &mockRoute{})
	if err := m.validateSettings(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV4_ValidateSettings_EmptyServer(t *testing.T) {
	s := settingsV4Only(t)
	s.Server = settings.Host{}
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for empty server")
	}
	assertContains(t, err.Error(), "empty Server")
}

func TestV4_ValidateSettings_InvalidIPv4(t *testing.T) {
	s := settingsV4Only(t)
	s.IPv4 = netip.Addr{}
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for invalid IPv4")
	}
	assertContains(t, err.Error(), "invalid IPv4")
}

func TestV4_ValidateSettings_IPv6AsIPv4(t *testing.T) {
	s := settingsV4Only(t)
	s.IPv4 = netip.MustParseAddr("fd00::2") // not Is4
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for IPv6 in IPv4 field")
	}
	assertContains(t, err.Error(), "invalid IPv4")
}

func TestV4_ValidateSettings_InvalidSubnet(t *testing.T) {
	s := settingsV4Only(t)
	s.IPv4Subnet = netip.Prefix{}
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for invalid subnet")
	}
	assertContains(t, err.Error(), "invalid IPv4Subnet")
}

// ---------------------------------------------------------------------------
// v6 validateSettings
// ---------------------------------------------------------------------------

func TestV6_ValidateSettings_Valid(t *testing.T) {
	m := newV6(settingsV6Only(t), &mockIfconfig{}, &mockRoute{})
	if err := m.validateSettings(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV6_ValidateSettings_EmptyServer(t *testing.T) {
	s := settingsV6Only(t)
	s.Server = settings.Host{}
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for empty server")
	}
	assertContains(t, err.Error(), "empty Server")
}

func TestV6_ValidateSettings_InvalidIPv6(t *testing.T) {
	s := settingsV6Only(t)
	s.IPv6 = netip.Addr{}
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for invalid IPv6")
	}
	assertContains(t, err.Error(), "invalid IPv6")
}

func TestV6_ValidateSettings_IPv4AsIPv6(t *testing.T) {
	s := settingsV6Only(t)
	s.IPv6 = netip.MustParseAddr("10.0.0.2") // Is4
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for IPv4 in IPv6 field")
	}
	assertContains(t, err.Error(), "invalid IPv6")
}

// ---------------------------------------------------------------------------
// dualStack validateSettings
// ---------------------------------------------------------------------------

func TestDualStack_ValidateSettings_Valid(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	if err := m.validateSettings(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDualStack_ValidateSettings_EmptyServer(t *testing.T) {
	s := settingsDualStack(t)
	s.Server = settings.Host{}
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for empty server")
	}
	assertContains(t, err.Error(), "empty Server")
}

func TestDualStack_ValidateSettings_InvalidIPv4(t *testing.T) {
	s := settingsDualStack(t)
	s.IPv4 = netip.Addr{}
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for invalid IPv4")
	}
	assertContains(t, err.Error(), "invalid IPv4")
}

func TestDualStack_ValidateSettings_InvalidIPv6(t *testing.T) {
	s := settingsDualStack(t)
	s.IPv6 = netip.Addr{}
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for invalid IPv6")
	}
	assertContains(t, err.Error(), "invalid IPv6")
}

func TestDualStack_ValidateSettings_IPv4InIPv6Field(t *testing.T) {
	s := settingsDualStack(t)
	s.IPv6 = netip.MustParseAddr("10.0.0.5") // Is4
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for IPv4 in IPv6 field")
	}
	assertContains(t, err.Error(), "invalid IPv6")
}

func TestDualStack_ValidateSettings_IPv6InIPv4Field(t *testing.T) {
	s := settingsDualStack(t)
	s.IPv4 = netip.MustParseAddr("fd00::99") // not Is4
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	err := m.validateSettings()
	if err == nil {
		t.Fatal("expected error for IPv6 in IPv4 field")
	}
	assertContains(t, err.Error(), "invalid IPv4")
}

// ---------------------------------------------------------------------------
// effectiveMTU
// ---------------------------------------------------------------------------

func TestV4_EffectiveMTU_UsesConfigured(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = 1400
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 1400 {
		t.Fatalf("expected 1400, got %d", got)
	}
}

func TestV4_EffectiveMTU_ZeroDefaultsToSafe(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = 0
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.SafeMTU {
		t.Fatalf("expected SafeMTU=%d, got %d", settings.SafeMTU, got)
	}
}

func TestV4_EffectiveMTU_NegativeDefaultsToSafe(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = -100
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.SafeMTU {
		t.Fatalf("expected SafeMTU=%d, got %d", settings.SafeMTU, got)
	}
}

func TestV4_EffectiveMTU_BelowMinimumClamped(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = settings.MinimumIPv4MTU - 1
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.MinimumIPv4MTU {
		t.Fatalf("expected MinimumIPv4MTU=%d, got %d", settings.MinimumIPv4MTU, got)
	}
}

func TestV4_EffectiveMTU_ExactMinimumAccepted(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = settings.MinimumIPv4MTU
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.MinimumIPv4MTU {
		t.Fatalf("expected %d, got %d", settings.MinimumIPv4MTU, got)
	}
}

func TestV6_EffectiveMTU_UsesConfigured(t *testing.T) {
	s := settingsV6Only(t)
	s.MTU = 1500
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 1500 {
		t.Fatalf("expected 1500, got %d", got)
	}
}

func TestV6_EffectiveMTU_ZeroDefaultsToSafe(t *testing.T) {
	s := settingsV6Only(t)
	s.MTU = 0
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	// SafeMTU < 1280, so clamped to 1280
	if got := m.effectiveMTU(); got != 1280 {
		t.Fatalf("expected 1280, got %d", got)
	}
}

func TestV6_EffectiveMTU_BelowMinimumClamped(t *testing.T) {
	s := settingsV6Only(t)
	s.MTU = 1000
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 1280 {
		t.Fatalf("expected 1280, got %d", got)
	}
}

func TestV6_EffectiveMTU_ExactMinimumAccepted(t *testing.T) {
	s := settingsV6Only(t)
	s.MTU = 1280
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 1280 {
		t.Fatalf("expected 1280, got %d", got)
	}
}

func TestDualStack_EffectiveMTU_UsesConfigured(t *testing.T) {
	s := settingsDualStack(t)
	s.MTU = 1400
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 1400 {
		t.Fatalf("expected 1400, got %d", got)
	}
}

func TestDualStack_EffectiveMTU_ZeroDefaultsAndClampsToIPv6Min(t *testing.T) {
	s := settingsDualStack(t)
	s.MTU = 0
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.MinimumIPv6MTU {
		t.Fatalf("expected MinimumIPv6MTU=%d, got %d", settings.MinimumIPv6MTU, got)
	}
}

func TestDualStack_EffectiveMTU_BelowIPv6MinClamped(t *testing.T) {
	s := settingsDualStack(t)
	s.MTU = 800
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.MinimumIPv6MTU {
		t.Fatalf("expected MinimumIPv6MTU=%d, got %d", settings.MinimumIPv6MTU, got)
	}
}

// ---------------------------------------------------------------------------
// resolveRouteIPv4 / resolveRouteIPv6
// ---------------------------------------------------------------------------

func TestV4_ResolveRouteIPv4_FromRouteEndpoint(t *testing.T) {
	m := newV4(settingsV4Only(t), &mockIfconfig{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("203.0.113.5:51820")
	ip, err := m.resolveRouteIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %s", ip)
	}
}

func TestV4_ResolveRouteIPv4_FromServer(t *testing.T) {
	m := newV4(settingsV4Only(t), &mockIfconfig{}, &mockRoute{})
	ip, err := m.resolveRouteIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "198.51.100.1" {
		t.Fatalf("expected 198.51.100.1, got %s", ip)
	}
}

func TestV4_ResolveRouteIPv4_IPv6EndpointErrors(t *testing.T) {
	m := newV4(settingsV4Only(t), &mockIfconfig{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("[2001:db8::1]:51820")
	_, err := m.resolveRouteIPv4()
	if err == nil {
		t.Fatal("expected error for IPv6 route endpoint in v4 context")
	}
	assertContains(t, err.Error(), "expected IPv4")
}

func TestV6_ResolveRouteIPv6_FromRouteEndpoint(t *testing.T) {
	m := newV6(settingsV6Only(t), &mockIfconfig{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("[2001:db8::99]:51820")
	ip, err := m.resolveRouteIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "2001:db8::99" {
		t.Fatalf("expected 2001:db8::99, got %s", ip)
	}
}

func TestV6_ResolveRouteIPv6_FromServer(t *testing.T) {
	m := newV6(settingsV6Only(t), &mockIfconfig{}, &mockRoute{})
	ip, err := m.resolveRouteIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %s", ip)
	}
}

func TestV6_ResolveRouteIPv6_IPv4EndpointErrors(t *testing.T) {
	m := newV6(settingsV6Only(t), &mockIfconfig{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("198.51.100.1:51820")
	_, err := m.resolveRouteIPv6()
	if err == nil {
		t.Fatal("expected error for IPv4 route endpoint in v6 context")
	}
	assertContains(t, err.Error(), "expected IPv6")
}

func TestDualStack_ResolveRouteIPv4_FromEndpoint(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("203.0.113.10:51820")
	ip, err := m.resolveRouteIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "203.0.113.10" {
		t.Fatalf("expected 203.0.113.10, got %s", ip)
	}
}

func TestDualStack_ResolveRouteIPv4_IPv6EndpointErrors(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("[2001:db8::1]:51820")
	_, err := m.resolveRouteIPv4()
	if err == nil {
		t.Fatal("expected error for IPv6 endpoint in v4 resolve")
	}
	assertContains(t, err.Error(), "expected IPv4")
}

func TestDualStack_ResolveRouteIPv6_FromEndpoint(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("[2001:db8::55]:51820")
	ip, err := m.resolveRouteIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "2001:db8::55" {
		t.Fatalf("expected 2001:db8::55, got %s", ip)
	}
}

func TestDualStack_ResolveRouteIPv6_IPv4EndpointErrors(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	m.routeEndpoint = netip.MustParseAddrPort("198.51.100.1:51820")
	_, err := m.resolveRouteIPv6()
	if err == nil {
		t.Fatal("expected error for IPv4 endpoint in v6 resolve")
	}
	assertContains(t, err.Error(), "expected IPv6")
}

func TestDualStack_ResolveRouteIPv4_FromServer(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	ip, err := m.resolveRouteIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "198.51.100.1" {
		t.Fatalf("expected 198.51.100.1, got %s", ip)
	}
}

func TestDualStack_ResolveRouteIPv6_FromIPv4OnlyServer_Errors(t *testing.T) {
	// Server is IPv4-only, so RouteIPv6 should fail
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	_, err := m.resolveRouteIPv6()
	if err == nil {
		t.Fatal("expected error resolving IPv6 from IPv4-only server")
	}
}

// ---------------------------------------------------------------------------
// shouldSkipDarwinIPv4Route / shouldSkipDarwinIPv6Route
// ---------------------------------------------------------------------------

func TestShouldSkipDarwinIPv4Route_NilError(t *testing.T) {
	if shouldSkipDarwinIPv4Route(nil) {
		t.Fatal("expected false for nil error")
	}
}

func TestShouldSkipDarwinIPv4Route_ExpectedIPv4(t *testing.T) {
	if !shouldSkipDarwinIPv4Route(fmt.Errorf("route endpoint ::1 is IPv6, expected IPv4")) {
		t.Fatal("expected true for 'expected IPv4' message")
	}
}

func TestShouldSkipDarwinIPv4Route_NoMatchingFamily(t *testing.T) {
	if !shouldSkipDarwinIPv4Route(fmt.Errorf("no matching address family found resolving host")) {
		t.Fatal("expected true for 'no matching address family found'")
	}
}

func TestShouldSkipDarwinIPv4Route_OtherError(t *testing.T) {
	if shouldSkipDarwinIPv4Route(fmt.Errorf("connection refused")) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestShouldSkipDarwinIPv6Route_NilError(t *testing.T) {
	if shouldSkipDarwinIPv6Route(nil) {
		t.Fatal("expected false for nil error")
	}
}

func TestShouldSkipDarwinIPv6Route_ExpectedIPv6(t *testing.T) {
	if !shouldSkipDarwinIPv6Route(fmt.Errorf("route endpoint 1.2.3.4 is IPv4, expected IPv6")) {
		t.Fatal("expected true for 'expected IPv6' message")
	}
}

func TestShouldSkipDarwinIPv6Route_NoMatchingFamily(t *testing.T) {
	if !shouldSkipDarwinIPv6Route(fmt.Errorf("no matching address family found resolving host")) {
		t.Fatal("expected true for 'no matching address family found'")
	}
}

func TestShouldSkipDarwinIPv6Route_OtherError(t *testing.T) {
	if shouldSkipDarwinIPv6Route(fmt.Errorf("timeout")) {
		t.Fatal("expected false for unrelated error")
	}
}

// ---------------------------------------------------------------------------
// SetRouteEndpoint
// ---------------------------------------------------------------------------

func TestV4_SetRouteEndpoint(t *testing.T) {
	m := newV4(settingsV4Only(t), &mockIfconfig{}, &mockRoute{})
	ep := netip.MustParseAddrPort("203.0.113.5:51820")
	m.SetRouteEndpoint(ep)
	if m.routeEndpoint != ep {
		t.Fatalf("expected routeEndpoint=%v, got %v", ep, m.routeEndpoint)
	}
}

func TestV6_SetRouteEndpoint(t *testing.T) {
	m := newV6(settingsV6Only(t), &mockIfconfig{}, &mockRoute{})
	ep := netip.MustParseAddrPort("[2001:db8::1]:51820")
	m.SetRouteEndpoint(ep)
	if m.routeEndpoint != ep {
		t.Fatalf("expected routeEndpoint=%v, got %v", ep, m.routeEndpoint)
	}
}

func TestDualStack_SetRouteEndpoint(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	ep := netip.MustParseAddrPort("203.0.113.5:51820")
	m.SetRouteEndpoint(ep)
	if m.routeEndpoint != ep {
		t.Fatalf("expected routeEndpoint=%v, got %v", ep, m.routeEndpoint)
	}
}

// ---------------------------------------------------------------------------
// CloseTunnel
// ---------------------------------------------------------------------------

func TestV4_CloseTunnel_CleanupRoutes(t *testing.T) {
	rt := &mockRoute{}
	m := newV4(settingsV4Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	m.resolvedRouteIP = "198.51.100.1"
	dev := &mockTunDevice{}
	m.tunDev = dev

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if len(rt.delSplitCalls) != 1 || rt.delSplitCalls[0] != "utun42" {
		t.Fatalf("expected DelSplit(utun42), got %v", rt.delSplitCalls)
	}
	if len(rt.delCalls) != 1 || rt.delCalls[0] != "198.51.100.1" {
		t.Fatalf("expected Del(198.51.100.1), got %v", rt.delCalls)
	}
	if !dev.closed {
		t.Fatal("expected tunDev.Close() to be called")
	}
	if m.tunDev != nil {
		t.Fatal("expected tunDev to be nil after dispose")
	}
	if m.ifName != "" {
		t.Fatal("expected ifName to be empty after dispose")
	}
}

func TestV4_CloseTunnel_NoResolvedRouteIP_SkipsDel(t *testing.T) {
	rt := &mockRoute{}
	m := newV4(settingsV4Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	m.resolvedRouteIP = ""

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if len(rt.delCalls) != 0 {
		t.Fatalf("expected no Del calls, got %v", rt.delCalls)
	}
}

func TestV4_CloseTunnel_RawUTUNFallback(t *testing.T) {
	rt := &mockRoute{}
	m := newV4(settingsV4Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	raw := &mockUTUN{}
	m.rawUTUN = raw
	// tunDev is nil, so raw should be closed directly

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if !raw.closed {
		t.Fatal("expected rawUTUN.Close() to be called when tunDev is nil")
	}
	if m.rawUTUN != nil {
		t.Fatal("expected rawUTUN to be nil after dispose")
	}
}

func TestV6_CloseTunnel_CleanupRoutes(t *testing.T) {
	rt := &mockRoute{}
	m := newV6(settingsV6Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	m.resolvedRouteIP = "2001:db8::1"
	dev := &mockTunDevice{}
	m.tunDev = dev

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if len(rt.delSplitCalls) != 1 || rt.delSplitCalls[0] != "utun42" {
		t.Fatalf("expected DelSplit(utun42), got %v", rt.delSplitCalls)
	}
	if len(rt.delCalls) != 1 || rt.delCalls[0] != "2001:db8::1" {
		t.Fatalf("expected Del(2001:db8::1), got %v", rt.delCalls)
	}
	if !dev.closed {
		t.Fatal("expected tunDev.Close() to be called")
	}
}

func TestV6_CloseTunnel_RawUTUNFallback(t *testing.T) {
	rt := &mockRoute{}
	m := newV6(settingsV6Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	raw := &mockUTUN{}
	m.rawUTUN = raw

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if !raw.closed {
		t.Fatal("expected rawUTUN.Close() to be called when tunDev is nil")
	}
}

func TestDualStack_CloseTunnel_CleanupAllRoutes(t *testing.T) {
	rt4 := &mockRoute{}
	rt6 := &mockRoute{}
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, rt4, rt6)
	m.ifName = "utun42"
	m.resolvedRouteIP4 = "198.51.100.1"
	m.resolvedRouteIP6 = "2001:db8::1"
	dev := &mockTunDevice{}
	m.tunDev = dev

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if len(rt4.delSplitCalls) != 1 || rt4.delSplitCalls[0] != "utun42" {
		t.Fatalf("expected v4 DelSplit(utun42), got %v", rt4.delSplitCalls)
	}
	if len(rt6.delSplitCalls) != 1 || rt6.delSplitCalls[0] != "utun42" {
		t.Fatalf("expected v6 DelSplit(utun42), got %v", rt6.delSplitCalls)
	}
	if len(rt4.delCalls) != 1 || rt4.delCalls[0] != "198.51.100.1" {
		t.Fatalf("expected v4 Del(198.51.100.1), got %v", rt4.delCalls)
	}
	if len(rt6.delCalls) != 1 || rt6.delCalls[0] != "2001:db8::1" {
		t.Fatalf("expected v6 Del(2001:db8::1), got %v", rt6.delCalls)
	}
	if !dev.closed {
		t.Fatal("expected tunDev.Close() to be called")
	}
	if m.tunDev != nil {
		t.Fatal("expected tunDev to be nil after dispose")
	}
	if m.rawUTUN != nil {
		t.Fatal("expected rawUTUN to be nil after dispose")
	}
	if m.ifName != "" {
		t.Fatal("expected ifName to be empty after dispose")
	}
}

func TestDualStack_CloseTunnel_NoResolvedIPs_SkipsDel(t *testing.T) {
	rt4 := &mockRoute{}
	rt6 := &mockRoute{}
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, rt4, rt6)
	m.ifName = "utun42"
	// No resolved IPs

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if len(rt4.delCalls) != 0 {
		t.Fatalf("expected no v4 Del calls, got %v", rt4.delCalls)
	}
	if len(rt6.delCalls) != 0 {
		t.Fatalf("expected no v6 Del calls, got %v", rt6.delCalls)
	}
}

func TestDualStack_CloseTunnel_RawUTUNFallback(t *testing.T) {
	rt4 := &mockRoute{}
	rt6 := &mockRoute{}
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, rt4, rt6)
	m.ifName = "utun42"
	raw := &mockUTUN{}
	m.rawUTUN = raw

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if !raw.closed {
		t.Fatal("expected rawUTUN.Close() to be called when tunDev is nil")
	}
}

func TestDualStack_CloseTunnel_OnlyV4Resolved(t *testing.T) {
	rt4 := &mockRoute{}
	rt6 := &mockRoute{}
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, rt4, rt6)
	m.ifName = "utun42"
	m.resolvedRouteIP4 = "198.51.100.1"
	m.resolvedRouteIP6 = ""

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}

	if len(rt4.delCalls) != 1 {
		t.Fatalf("expected 1 v4 Del call, got %d", len(rt4.delCalls))
	}
	if len(rt6.delCalls) != 0 {
		t.Fatalf("expected 0 v6 Del calls, got %d", len(rt6.delCalls))
	}
}

// ---------------------------------------------------------------------------
// v4 assignIPv4
// ---------------------------------------------------------------------------

func TestV4_AssignIPv4_Success(t *testing.T) {
	ifc := &mockIfconfig{}
	m := newV4(settingsV4Only(t), ifc, &mockRoute{})
	m.ifName = "utun42"

	err := m.assignIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ifc.linkAddrAddCalls) != 1 {
		t.Fatalf("expected 1 LinkAddrAdd call, got %d", len(ifc.linkAddrAddCalls))
	}
	if ifc.linkAddrAddCalls[0].cidr != "10.0.0.2/32" {
		t.Fatalf("expected cidr=10.0.0.2/32, got %s", ifc.linkAddrAddCalls[0].cidr)
	}
	if ifc.linkAddrAddCalls[0].ifName != "utun42" {
		t.Fatalf("expected ifName=utun42, got %s", ifc.linkAddrAddCalls[0].ifName)
	}
}

func TestV4_AssignIPv4_Error(t *testing.T) {
	ifc := &mockIfconfig{linkAddrAddErr: fmt.Errorf("ifconfig failed")}
	m := newV4(settingsV4Only(t), ifc, &mockRoute{})
	m.ifName = "utun42"

	err := m.assignIPv4()
	if err == nil {
		t.Fatal("expected error from LinkAddrAdd failure")
	}
	assertContains(t, err.Error(), "set addr")
}

// ---------------------------------------------------------------------------
// v6 assignIPv6
// ---------------------------------------------------------------------------

func TestV6_AssignIPv6_WithSubnet(t *testing.T) {
	ifc := &mockIfconfig{}
	m := newV6(settingsV6Only(t), ifc, &mockRoute{})
	m.ifName = "utun42"

	err := m.assignIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ifc.linkAddrAddCalls) != 1 {
		t.Fatalf("expected 1 LinkAddrAdd call, got %d", len(ifc.linkAddrAddCalls))
	}
	if ifc.linkAddrAddCalls[0].cidr != "fd00::2/64" {
		t.Fatalf("expected cidr=fd00::2/64, got %s", ifc.linkAddrAddCalls[0].cidr)
	}
}

func TestV6_AssignIPv6_WithoutSubnet_Defaults128(t *testing.T) {
	s := settingsV6Only(t)
	s.IPv6Subnet = netip.Prefix{} // invalid
	ifc := &mockIfconfig{}
	m := newV6(s, ifc, &mockRoute{})
	m.ifName = "utun42"

	err := m.assignIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ifc.linkAddrAddCalls[0].cidr != "fd00::2/128" {
		t.Fatalf("expected cidr=fd00::2/128, got %s", ifc.linkAddrAddCalls[0].cidr)
	}
}

func TestV6_AssignIPv6_Error(t *testing.T) {
	ifc := &mockIfconfig{linkAddrAddErr: fmt.Errorf("ifconfig failed")}
	m := newV6(settingsV6Only(t), ifc, &mockRoute{})
	m.ifName = "utun42"

	err := m.assignIPv6()
	if err == nil {
		t.Fatal("expected error from LinkAddrAdd failure")
	}
	assertContains(t, err.Error(), "set addr")
}

// ---------------------------------------------------------------------------
// newV4 / newV6 / newDualStack constructor checks
// ---------------------------------------------------------------------------

func TestNewV4_StoresFields(t *testing.T) {
	s := settingsV4Only(t)
	ifc := &mockIfconfig{}
	rt := &mockRoute{}
	m := newV4(s, ifc, rt)
	if m.s.MTU != s.MTU || m.s.IPv4 != s.IPv4 {
		t.Fatal("settings not stored")
	}
	if m.ifc != ifc {
		t.Fatal("ifconfig not stored")
	}
	if m.rtc != rt {
		t.Fatal("route not stored")
	}
}

func TestNewV6_StoresFields(t *testing.T) {
	s := settingsV6Only(t)
	ifc := &mockIfconfig{}
	rt := &mockRoute{}
	m := newV6(s, ifc, rt)
	if m.s.MTU != s.MTU || m.s.IPv6 != s.IPv6 {
		t.Fatal("settings not stored")
	}
	if m.ifc != ifc {
		t.Fatal("ifconfig not stored")
	}
	if m.rt != rt {
		t.Fatal("route not stored")
	}
}

func TestNewDualStack_StoresFields(t *testing.T) {
	s := settingsDualStack(t)
	ifc4 := &mockIfconfig{}
	ifc6 := &mockIfconfig{}
	rt4 := &mockRoute{}
	rt6 := &mockRoute{}
	m := newDualStack(s, ifc4, ifc6, rt4, rt6)
	if m.s.MTU != s.MTU || m.s.IPv4 != s.IPv4 || m.s.IPv6 != s.IPv6 {
		t.Fatal("settings not stored")
	}
	if m.ifc4 != ifc4 || m.ifc6 != ifc6 {
		t.Fatal("ifconfigs not stored correctly")
	}
	if m.rtc4 != rt4 || m.rtc6 != rt6 {
		t.Fatal("routes not stored correctly")
	}
}

// ---------------------------------------------------------------------------
// CloseTunnel idempotency: disposing a fresh (never-used) manager is safe
// ---------------------------------------------------------------------------

func TestV4_CloseTunnel_Fresh(t *testing.T) {
	m := newV4(settingsV4Only(t), &mockIfconfig{}, &mockRoute{})
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error disposing fresh v4: %v", err)
	}
}

func TestV6_CloseTunnel_Fresh(t *testing.T) {
	m := newV6(settingsV6Only(t), &mockIfconfig{}, &mockRoute{})
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error disposing fresh v6: %v", err)
	}
}

func TestDualStack_CloseTunnel_Fresh(t *testing.T) {
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error disposing fresh dualStack: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DualStack: server with both IPv4 and IPv6
// ---------------------------------------------------------------------------

func TestDualStack_ResolveRouteIPv6_FromDualStackServer(t *testing.T) {
	host := mustIPHost(t, "198.51.100.1")
	host.IPv6 = "2001:db8::1"
	s := settings.Settings{
		Addressing: settings.Addressing{
			Server:     host,
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
		},
		MTU: 1400,
	}
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	ip, err := m.resolveRouteIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %s", ip)
	}
}

// ---------------------------------------------------------------------------
// CloseTunnel double-call safety
// ---------------------------------------------------------------------------

func TestV4_CloseTunnel_DoubleSafe(t *testing.T) {
	rt := &mockRoute{}
	m := newV4(settingsV4Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	m.resolvedRouteIP = "198.51.100.1"
	m.tunDev = &mockTunDevice{}

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}
	// Second call should not panic or fail
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error on double dispose: %v", err)
	}
}

func TestV6_CloseTunnel_DoubleSafe(t *testing.T) {
	rt := &mockRoute{}
	m := newV6(settingsV6Only(t), &mockIfconfig{}, rt)
	m.ifName = "utun42"
	m.resolvedRouteIP = "2001:db8::1"
	m.tunDev = &mockTunDevice{}

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error on double dispose: %v", err)
	}
}

func TestDualStack_CloseTunnel_DoubleSafe(t *testing.T) {
	rt4 := &mockRoute{}
	rt6 := &mockRoute{}
	m := newDualStack(settingsDualStack(t), &mockIfconfig{}, &mockIfconfig{}, rt4, rt6)
	m.ifName = "utun42"
	m.resolvedRouteIP4 = "198.51.100.1"
	m.resolvedRouteIP6 = "2001:db8::1"
	m.tunDev = &mockTunDevice{}

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected dispose error: %v", err)
	}
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error on double dispose: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Edge cases for effectiveMTU
// ---------------------------------------------------------------------------

func TestV4_EffectiveMTU_LargeValue(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = 9000
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 9000 {
		t.Fatalf("expected 9000, got %d", got)
	}
}

func TestV6_EffectiveMTU_LargeValue(t *testing.T) {
	s := settingsV6Only(t)
	s.MTU = 9000
	m := newV6(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 9000 {
		t.Fatalf("expected 9000, got %d", got)
	}
}

func TestDualStack_EffectiveMTU_LargeValue(t *testing.T) {
	s := settingsDualStack(t)
	s.MTU = 9000
	m := newDualStack(s, &mockIfconfig{}, &mockIfconfig{}, &mockRoute{}, &mockRoute{})
	if got := m.effectiveMTU(); got != 9000 {
		t.Fatalf("expected 9000, got %d", got)
	}
}

func TestV4_EffectiveMTU_ExactSafeMTU(t *testing.T) {
	s := settingsV4Only(t)
	s.MTU = settings.SafeMTU
	m := newV4(s, &mockIfconfig{}, &mockRoute{})
	if got := m.effectiveMTU(); got != settings.SafeMTU {
		t.Fatalf("expected %d, got %d", settings.SafeMTU, got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if len(s) == 0 {
		t.Fatalf("string is empty, expected it to contain %q", substr)
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	t.Fatalf("string %q does not contain %q", s, substr)
}
