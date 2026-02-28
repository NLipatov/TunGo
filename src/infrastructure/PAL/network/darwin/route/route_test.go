//go:build darwin

package route

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock commander
// ---------------------------------------------------------------------------

type call struct {
	name string
	args []string
}

type stubbedResult struct {
	out []byte
	err error
}

// mockCommander records every invocation and returns pre-configured results
// keyed by the full command string ("route -n get 8.8.8.8").
type mockCommander struct {
	mu    sync.Mutex
	calls []call
	// stubs maps "name arg0 arg1 ..." to a list of results consumed in order.
	stubs map[string][]stubbedResult
	// defaultResult is returned when no stub matches.
	defaultResult stubbedResult
}

func newMockCommander() *mockCommander {
	return &mockCommander{
		stubs: make(map[string][]stubbedResult),
	}
}

func (m *mockCommander) key(name string, args ...string) string {
	parts := append([]string{name}, args...)
	return strings.Join(parts, " ")
}

func (m *mockCommander) stub(out []byte, err error, name string, args ...string) {
	k := m.key(name, args...)
	m.stubs[k] = append(m.stubs[k], stubbedResult{out: out, err: err})
}

func (m *mockCommander) lookup(name string, args ...string) stubbedResult {
	k := m.key(name, args...)
	m.mu.Lock()
	defer m.mu.Unlock()
	if results, ok := m.stubs[k]; ok && len(results) > 0 {
		r := results[0]
		if len(results) > 1 {
			m.stubs[k] = results[1:]
		}
		// keep last result for repeated calls
		return r
	}
	return m.defaultResult
}

func (m *mockCommander) record(name string, args ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call{name: name, args: append([]string{}, args...)})
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.record(name, args...)
	r := m.lookup(name, args...)
	return r.out, r.err
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	m.record(name, args...)
	r := m.lookup(name, args...)
	return r.out, r.err
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.record(name, args...)
	r := m.lookup(name, args...)
	return r.err
}

func (m *mockCommander) allCalls() []call {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]call, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *mockCommander) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// hasCall returns true if any recorded call matches exactly.
func (m *mockCommander) hasCall(name string, args ...string) bool {
	for _, c := range m.allCalls() {
		if c.name != name || len(c.args) != len(args) {
			continue
		}
		match := true
		for i := range args {
			if c.args[i] != args[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helper to build typical `route -n get` output.
// ---------------------------------------------------------------------------

func routeGetOutput(gateway, iface string) []byte {
	var sb strings.Builder
	sb.WriteString("   route to: 8.8.8.8\n")
	sb.WriteString("destination: default\n")
	if gateway != "" {
		sb.WriteString(fmt.Sprintf("    gateway: %s\n", gateway))
	}
	if iface != "" {
		sb.WriteString(fmt.Sprintf("  interface: %s\n", iface))
	}
	sb.WriteString("      flags: <UP,GATEWAY,DONE>\n")
	return []byte(sb.String())
}

// ---------------------------------------------------------------------------
// Factory tests
// ---------------------------------------------------------------------------

func TestNewFactory(t *testing.T) {
	cmd := newMockCommander()
	f := NewFactory(cmd)
	if f == nil {
		t.Fatal("NewFactory returned nil")
	}
}

func TestFactory_NewV4(t *testing.T) {
	cmd := newMockCommander()
	f := NewFactory(cmd)
	c := f.NewV4()
	if c == nil {
		t.Fatal("NewV4 returned nil")
	}
	// Verify it satisfies Contract.
	var _ Contract = c
}

func TestFactory_NewV6(t *testing.T) {
	cmd := newMockCommander()
	f := NewFactory(cmd)
	c := f.NewV6()
	if c == nil {
		t.Fatal("NewV6 returned nil")
	}
	var _ Contract = c
}

// ---------------------------------------------------------------------------
// v4 parseRoute tests
// ---------------------------------------------------------------------------

func TestV4_parseRoute_gatewayAndInterface(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutput("192.168.1.1", "en0"), nil, "route", "-n", "get", "8.8.8.8")

	r := newV4(cmd).(*v4)
	gw, iface, err := r.parseRoute("8.8.8.8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "192.168.1.1" {
		t.Errorf("gateway = %q, want %q", gw, "192.168.1.1")
	}
	if iface != "en0" {
		t.Errorf("interface = %q, want %q", iface, "en0")
	}
}

func TestV4_parseRoute_interfaceOnly(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutput("", "utun3"), nil, "route", "-n", "get", "10.0.0.1")

	r := newV4(cmd).(*v4)
	gw, iface, err := r.parseRoute("10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "" {
		t.Errorf("gateway = %q, want empty", gw)
	}
	if iface != "utun3" {
		t.Errorf("interface = %q, want %q", iface, "utun3")
	}
}

func TestV4_parseRoute_noMatches(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("some unrelated output\n"), nil, "route", "-n", "get", "1.2.3.4")

	r := newV4(cmd).(*v4)
	gw, iface, err := r.parseRoute("1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "" || iface != "" {
		t.Errorf("expected empty gw/iface, got gw=%q iface=%q", gw, iface)
	}
}

func TestV4_parseRoute_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("error output"), errors.New("exit 1"), "route", "-n", "get", "8.8.8.8")

	r := newV4(cmd).(*v4)
	_, _, err := r.parseRoute("8.8.8.8")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// v4 Get tests
// ---------------------------------------------------------------------------

func TestV4_Get_validIPWithGateway(t *testing.T) {
	cmd := newMockCommander()
	// route get for the destination returns a real gateway
	cmd.stub(routeGetOutput("192.168.1.1", "en0"), nil, "route", "-n", "get", "8.8.8.8")
	// deleteQuiet
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "8.8.8.8")
	// addViaGatewayQuiet
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "8.8.8.8", "192.168.1.1")

	r := newV4(cmd)
	if err := r.Get("8.8.8.8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify delete was called before add
	if !cmd.hasCall("route", "-q", "-n", "delete", "8.8.8.8") {
		t.Error("expected delete call for old route")
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "8.8.8.8", "192.168.1.1") {
		t.Error("expected add via gateway call")
	}
}

func TestV4_Get_validIPWithLinkInterface(t *testing.T) {
	cmd := newMockCommander()
	// "link#" prefix in gateway means on-link — should add via interface instead
	cmd.stub(routeGetOutput("link#14", "en0"), nil, "route", "-n", "get", "10.0.0.1")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.0.0.1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "10.0.0.1", "-interface", "en0")

	r := newV4(cmd)
	if err := r.Get("10.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "10.0.0.1", "-interface", "en0") {
		t.Error("expected add on-link call via interface")
	}
}

func TestV4_Get_loopbackFallsBackToDefault(t *testing.T) {
	cmd := newMockCommander()
	// First route get returns loopback
	cmd.stub(routeGetOutput("127.0.0.1", "lo0"), nil, "route", "-n", "get", "8.8.8.8")
	// Default route returns a real gateway
	cmd.stub(routeGetOutput("10.0.0.1", "en0"), nil, "route", "-n", "get", "default")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "8.8.8.8")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "8.8.8.8", "10.0.0.1")

	r := newV4(cmd)
	if err := r.Get("8.8.8.8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "8.8.8.8", "10.0.0.1") {
		t.Error("expected add via default gateway")
	}
}

func TestV4_Get_defaultAlsoLoopback(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutput("127.0.0.1", "lo0"), nil, "route", "-n", "get", "8.8.8.8")
	cmd.stub(routeGetOutput("127.0.0.1", "lo0"), nil, "route", "-n", "get", "default")

	r := newV4(cmd)
	err := r.Get("8.8.8.8")
	if err == nil {
		t.Fatal("expected error for all-loopback routes")
	}
	if !strings.Contains(err.Error(), "no non-loopback route") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV4_Get_invalidIP(t *testing.T) {
	cmd := newMockCommander()
	r := newV4(cmd)
	err := r.Get("not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
	if !strings.Contains(err.Error(), "invalid IP") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV4_Get_ipv6PassedToV4(t *testing.T) {
	cmd := newMockCommander()
	r := newV4(cmd)
	err := r.Get("2001:db8::1")
	if err == nil {
		t.Fatal("expected error for IPv6 passed to v4")
	}
	if !strings.Contains(err.Error(), "non-IPv4") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV4_Get_loopbackIP(t *testing.T) {
	cmd := newMockCommander()
	r := newV4(cmd)
	err := r.Get("127.0.0.1")
	if err == nil {
		t.Fatal("expected error for loopback IP")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV4_Get_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("network unreachable"), errors.New("exit 1"),
		"route", "-n", "get", "8.8.8.8")

	r := newV4(cmd)
	err := r.Get("8.8.8.8")
	if err == nil {
		t.Fatal("expected error when commander fails")
	}
}

// ---------------------------------------------------------------------------
// v4 Add / AddViaGateway / Del tests
// ---------------------------------------------------------------------------

func TestV4_Add_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.1.0.0")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "10.1.0.0", "-interface", "utun3")

	r := newV4(cmd)
	if err := r.Add("10.1.0.0", "utun3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "delete", "10.1.0.0") {
		t.Error("expected delete before add")
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "10.1.0.0", "-interface", "utun3") {
		t.Error("expected add on-link call")
	}
}

func TestV4_Add_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.1.0.0")
	cmd.stub([]byte("some error"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "10.1.0.0", "-interface", "utun3")

	r := newV4(cmd)
	if err := r.Add("10.1.0.0", "utun3"); err == nil {
		t.Fatal("expected error")
	}
}

func TestV4_AddViaGateway_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.1.0.0")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "10.1.0.0", "192.168.1.1")

	r := newV4(cmd)
	if err := r.AddViaGateway("10.1.0.0", "192.168.1.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV4_AddViaGateway_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.1.0.0")
	cmd.stub([]byte("error"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "10.1.0.0", "192.168.1.1")

	r := newV4(cmd)
	if err := r.AddViaGateway("10.1.0.0", "192.168.1.1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestV4_Del_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.1.0.0")

	r := newV4(cmd)
	if err := r.Del("10.1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV4_Del_notInTable(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "10.1.0.0")

	r := newV4(cmd)
	// "not in table" should be silently ignored
	if err := r.Del("10.1.0.0"); err != nil {
		t.Fatalf("expected nil error for 'not in table', got: %v", err)
	}
}

func TestV4_Del_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("permission denied"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "10.1.0.0")

	r := newV4(cmd)
	if err := r.Del("10.1.0.0"); err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v4 AddSplit / DelSplit tests
// ---------------------------------------------------------------------------

func TestV4_AddSplit_success(t *testing.T) {
	cmd := newMockCommander()
	// Pre-delete stubs (ignored errors)
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitTwo, "-interface", "utun3")
	// Add stubs
	cmd.stub(nil, nil,
		"route", "-q", "-n", "add", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "add", "-net", v4SplitTwo, "-interface", "utun3")

	r := newV4(cmd)
	if err := r.AddSplit("utun3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV4_AddSplit_fileExistsIgnored(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitTwo, "-interface", "utun3")
	// "File exists" should be silently ignored
	cmd.stub([]byte("File exists"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "add", "-net", v4SplitTwo, "-interface", "utun3")

	r := newV4(cmd)
	if err := r.AddSplit("utun3"); err != nil {
		t.Fatalf("expected nil for File exists, got: %v", err)
	}
}

func TestV4_AddSplit_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitTwo, "-interface", "utun3")
	cmd.stub([]byte("permission denied"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "-net", v4SplitOne, "-interface", "utun3")

	r := newV4(cmd)
	if err := r.AddSplit("utun3"); err == nil {
		t.Fatal("expected error")
	}
}

func TestV4_DelSplit_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil,
		"route", "-q", "-n", "delete", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "delete", "-net", v4SplitTwo, "-interface", "utun3")

	r := newV4(cmd)
	if err := r.DelSplit("utun3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV4_DelSplit_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("kernel error"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-net", v4SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "delete", "-net", v4SplitTwo, "-interface", "utun3")

	r := newV4(cmd)
	if err := r.DelSplit("utun3"); err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v4 DefaultGateway tests
// ---------------------------------------------------------------------------

func TestV4_DefaultGateway_found(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutput("192.168.1.1", "en0"), nil, "route", "-n", "get", "default")

	r := newV4(cmd)
	gw, err := r.DefaultGateway()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "192.168.1.1" {
		t.Errorf("gateway = %q, want %q", gw, "192.168.1.1")
	}
}

func TestV4_DefaultGateway_noGateway(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutput("", "en0"), nil, "route", "-n", "get", "default")

	r := newV4(cmd)
	_, err := r.DefaultGateway()
	if err == nil {
		t.Fatal("expected error when no gateway in output")
	}
	if !strings.Contains(err.Error(), "no gateway found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV4_DefaultGateway_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("error"), errors.New("exit 1"), "route", "-n", "get", "default")

	r := newV4(cmd)
	_, err := r.DefaultGateway()
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v6 parseRoute tests
// ---------------------------------------------------------------------------

func routeGetOutputV6(gateway, iface string) []byte {
	var sb strings.Builder
	sb.WriteString("   route to: 2001:db8::1\n")
	sb.WriteString("destination: default\n")
	if gateway != "" {
		sb.WriteString(fmt.Sprintf("    gateway: %s\n", gateway))
	}
	if iface != "" {
		sb.WriteString(fmt.Sprintf("  interface: %s\n", iface))
	}
	sb.WriteString("      flags: <UP,GATEWAY,DONE>\n")
	return []byte(sb.String())
}

func TestV6_parseRoute_gatewayAndInterface(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutputV6("fe80::1", "en0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")

	r := newV6(cmd).(*v6)
	gw, iface, err := r.parseRoute("2001:db8::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "fe80::1" {
		t.Errorf("gateway = %q, want %q", gw, "fe80::1")
	}
	if iface != "en0" {
		t.Errorf("interface = %q, want %q", iface, "en0")
	}
}

func TestV6_parseRoute_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("error"), errors.New("exit 1"),
		"route", "-n", "-inet6", "get", "2001:db8::1")

	r := newV6(cmd).(*v6)
	_, _, err := r.parseRoute("2001:db8::1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v6 Get tests
// ---------------------------------------------------------------------------

func TestV6_Get_validIPWithGateway(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutputV6("2001:db8::gw", "en0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "2001:db8::gw")

	r := newV6(cmd)
	if err := r.Get("2001:db8::1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "-inet6", "2001:db8::1", "2001:db8::gw") {
		t.Error("expected add via gateway call")
	}
}

func TestV6_Get_linkLocalWithoutScope(t *testing.T) {
	cmd := newMockCommander()
	// Gateway is link-local without % scope -> should append %en0
	cmd.stub(routeGetOutputV6("fe80::1", "en0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::1%en0")

	r := newV6(cmd)
	if err := r.Get("2001:db8::1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::1%en0") {
		t.Error("expected add via gateway with appended scope")
	}
}

func TestV6_Get_linkLocalWithScope(t *testing.T) {
	cmd := newMockCommander()
	// Gateway already has scope -> should not modify
	cmd.stub(routeGetOutputV6("fe80::1%en0", "en0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::1%en0")

	r := newV6(cmd)
	if err := r.Get("2001:db8::1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::1%en0") {
		t.Error("expected add via gateway with existing scope")
	}
}

func TestV6_Get_loopbackFallsBackToDefault(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutputV6("::1", "lo0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub(routeGetOutputV6("fe80::gw", "en0"), nil,
		"route", "-n", "-inet6", "get", "default")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::gw%en0")

	r := newV6(cmd)
	if err := r.Get("2001:db8::1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV6_Get_defaultAlsoLoopback(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutputV6("::1", "lo0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub(routeGetOutputV6("::1", "lo0"), nil,
		"route", "-n", "-inet6", "get", "default")

	r := newV6(cmd)
	err := r.Get("2001:db8::1")
	if err == nil {
		t.Fatal("expected error for all-loopback routes")
	}
	if !strings.Contains(err.Error(), "no non-loopback route") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV6_Get_nonIPv6(t *testing.T) {
	cmd := newMockCommander()
	r := newV6(cmd)
	err := r.Get("8.8.8.8")
	if err == nil {
		t.Fatal("expected error for IPv4 passed to v6")
	}
	if !strings.Contains(err.Error(), "non-IPv6") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV6_Get_invalidIP(t *testing.T) {
	cmd := newMockCommander()
	r := newV6(cmd)
	err := r.Get("not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
	if !strings.Contains(err.Error(), "invalid IP") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV6_Get_loopbackIP(t *testing.T) {
	cmd := newMockCommander()
	r := newV6(cmd)
	err := r.Get("::1")
	if err == nil {
		t.Fatal("expected error for loopback IP")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV6_Get_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("error"), errors.New("exit 1"),
		"route", "-n", "-inet6", "get", "2001:db8::1")

	r := newV6(cmd)
	err := r.Get("2001:db8::1")
	if err == nil {
		t.Fatal("expected error when commander fails")
	}
}

func TestV6_Get_interfaceOnlyViaLink(t *testing.T) {
	cmd := newMockCommander()
	// gateway is link#, so should fall through to add on-link via interface
	cmd.stub(routeGetOutputV6("link#14", "en0"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "-interface", "en0")

	r := newV6(cmd)
	if err := r.Get("2001:db8::1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "-inet6", "2001:db8::1", "-interface", "en0") {
		t.Error("expected add on-link via interface for link# gateway")
	}
}

// ---------------------------------------------------------------------------
// v6 Add / AddViaGateway / Del tests
// ---------------------------------------------------------------------------

func TestV6_Add_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "-interface", "utun3")

	r := newV6(cmd)
	if err := r.Add("2001:db8::1", "utun3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "delete", "-inet6", "2001:db8::1") {
		t.Error("expected delete before add")
	}
}

func TestV6_Add_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub([]byte("error"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "-inet6", "2001:db8::1", "-interface", "utun3")

	r := newV6(cmd)
	if err := r.Add("2001:db8::1", "utun3"); err == nil {
		t.Fatal("expected error")
	}
}

func TestV6_AddViaGateway_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::1%en0")

	r := newV6(cmd)
	if err := r.AddViaGateway("2001:db8::1", "fe80::1%en0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV6_AddViaGateway_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")
	cmd.stub([]byte("error"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "-inet6", "2001:db8::1", "fe80::1%en0")

	r := newV6(cmd)
	if err := r.AddViaGateway("2001:db8::1", "fe80::1%en0"); err == nil {
		t.Fatal("expected error")
	}
}

func TestV6_Del_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")

	r := newV6(cmd)
	if err := r.Del("2001:db8::1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV6_Del_notInTable(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", "2001:db8::1")

	r := newV6(cmd)
	if err := r.Del("2001:db8::1"); err != nil {
		t.Fatalf("expected nil for 'not in table', got: %v", err)
	}
}

func TestV6_Del_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("permission denied"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", "2001:db8::1")

	r := newV6(cmd)
	if err := r.Del("2001:db8::1"); err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v6 AddSplit / DelSplit tests
// ---------------------------------------------------------------------------

func TestV6_AddSplit_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitTwo, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "add", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "add", "-inet6", v6SplitTwo, "-interface", "utun3")

	r := newV6(cmd)
	if err := r.AddSplit("utun3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV6_AddSplit_fileExistsIgnored(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitTwo, "-interface", "utun3")
	cmd.stub([]byte("File exists"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "add", "-inet6", v6SplitTwo, "-interface", "utun3")

	r := newV6(cmd)
	if err := r.AddSplit("utun3"); err != nil {
		t.Fatalf("expected nil for File exists, got: %v", err)
	}
}

func TestV6_AddSplit_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub([]byte("not in table"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitTwo, "-interface", "utun3")
	cmd.stub([]byte("permission denied"), errors.New("exit 1"),
		"route", "-q", "-n", "add", "-inet6", v6SplitOne, "-interface", "utun3")

	r := newV6(cmd)
	if err := r.AddSplit("utun3"); err == nil {
		t.Fatal("expected error")
	}
}

func TestV6_DelSplit_success(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(nil, nil,
		"route", "-q", "-n", "delete", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "delete", "-inet6", v6SplitTwo, "-interface", "utun3")

	r := newV6(cmd)
	if err := r.DelSplit("utun3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV6_DelSplit_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("kernel error"), errors.New("exit 1"),
		"route", "-q", "-n", "delete", "-inet6", v6SplitOne, "-interface", "utun3")
	cmd.stub(nil, nil,
		"route", "-q", "-n", "delete", "-inet6", v6SplitTwo, "-interface", "utun3")

	r := newV6(cmd)
	if err := r.DelSplit("utun3"); err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v6 DefaultGateway tests
// ---------------------------------------------------------------------------

func TestV6_DefaultGateway_found(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutputV6("fe80::1", "en0"), nil,
		"route", "-n", "-inet6", "get", "default")

	r := newV6(cmd)
	gw, err := r.DefaultGateway()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "fe80::1" {
		t.Errorf("gateway = %q, want %q", gw, "fe80::1")
	}
}

func TestV6_DefaultGateway_noGateway(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutputV6("", "en0"), nil,
		"route", "-n", "-inet6", "get", "default")

	r := newV6(cmd)
	_, err := r.DefaultGateway()
	if err == nil {
		t.Fatal("expected error when no gateway in output")
	}
	if !strings.Contains(err.Error(), "no gateway found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestV6_DefaultGateway_commanderError(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("error"), errors.New("exit 1"),
		"route", "-n", "-inet6", "get", "default")

	r := newV6(cmd)
	_, err := r.DefaultGateway()
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// v4 Get — table-driven edge cases
// ---------------------------------------------------------------------------

func TestV4_Get_tableValidation(t *testing.T) {
	tests := []struct {
		name    string
		dest    string
		wantErr string
	}{
		{name: "empty string", dest: "", wantErr: "invalid IP"},
		{name: "garbage", dest: "abc.def.ghi", wantErr: "invalid IP"},
		{name: "IPv6 loopback", dest: "::1", wantErr: "non-IPv4"},
		{name: "IPv6 global", dest: "2001:db8::1", wantErr: "non-IPv4"},
		{name: "loopback 127.0.0.1", dest: "127.0.0.1", wantErr: "loopback"},
		{name: "loopback 127.1.2.3", dest: "127.1.2.3", wantErr: "loopback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newMockCommander()
			r := newV4(cmd)
			err := r.Get(tt.dest)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// v6 Get — table-driven edge cases
// ---------------------------------------------------------------------------

func TestV6_Get_tableValidation(t *testing.T) {
	tests := []struct {
		name    string
		dest    string
		wantErr string
	}{
		{name: "empty string", dest: "", wantErr: "invalid IP"},
		{name: "garbage", dest: "not-an-ip", wantErr: "invalid IP"},
		{name: "IPv4", dest: "8.8.8.8", wantErr: "non-IPv6"},
		{name: "loopback ::1", dest: "::1", wantErr: "loopback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newMockCommander()
			r := newV6(cmd)
			err := r.Get(tt.dest)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// v4 Get — empty route with no interface (no route found)
// ---------------------------------------------------------------------------

func TestV4_Get_noRouteFound(t *testing.T) {
	cmd := newMockCommander()
	// parseRoute returns empty gw and iface
	cmd.stub([]byte("some output with no gateway or interface lines\n"), nil,
		"route", "-n", "get", "8.8.8.8")
	// default route also has no usable info
	cmd.stub([]byte("some output with no gateway or interface lines\n"), nil,
		"route", "-n", "get", "default")
	// deleteQuiet is called but that's fine
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "8.8.8.8")

	r := newV4(cmd)
	err := r.Get("8.8.8.8")
	if err == nil {
		t.Fatal("expected error when no route found")
	}
	if !strings.Contains(err.Error(), "no route found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// v6 Get — empty route with no interface (no route found)
// ---------------------------------------------------------------------------

func TestV6_Get_noRouteFound(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub([]byte("some output\n"), nil,
		"route", "-n", "-inet6", "get", "2001:db8::1")
	cmd.stub([]byte("some output\n"), nil,
		"route", "-n", "-inet6", "get", "default")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "-inet6", "2001:db8::1")

	r := newV6(cmd)
	err := r.Get("2001:db8::1")
	if err == nil {
		t.Fatal("expected error when no route found")
	}
	if !strings.Contains(err.Error(), "no route found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// v4 Get — interface-only (no gateway) adds on-link
// ---------------------------------------------------------------------------

func TestV4_Get_interfaceOnlyNoGateway(t *testing.T) {
	cmd := newMockCommander()
	cmd.stub(routeGetOutput("", "en0"), nil, "route", "-n", "get", "10.0.0.5")
	cmd.stub(nil, nil, "route", "-q", "-n", "delete", "10.0.0.5")
	cmd.stub(nil, nil, "route", "-q", "-n", "add", "10.0.0.5", "-interface", "en0")

	r := newV4(cmd)
	if err := r.Get("10.0.0.5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmd.hasCall("route", "-q", "-n", "add", "10.0.0.5", "-interface", "en0") {
		t.Error("expected add on-link call via interface when no gateway")
	}
}
