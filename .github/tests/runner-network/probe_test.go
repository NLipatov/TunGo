package runnerprobe

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	config "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/client"
	"tungo/internal/tun/server"
)

func TestClientNetwork(t *testing.T) {
	requireRunner(t)
	configuration := &config.Configuration{
		ClientID: 1, Protocol: settings.UDP,
		UDPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "c_probe0",
				IPv4Subnet: netip.MustParsePrefix("198.19.254.0/24"),
				IPv6Subnet: netip.MustParsePrefix("fd73:7467:6f:ff::/64"),
			},
			MTU: settings.DefaultMTU,
		},
	}
	active, err := configuration.ActiveSettings()
	if err != nil {
		t.Fatal(err)
	}
	// UDP connect resolves the kernel route without sending a packet. These
	// destinations cover both halves of each address family, outside the TUN subnet.
	destinations := []string{"1.1.1.1", "203.0.113.1", "2001:db8::1", "fd00:1234::1"}
	before := make([]string, len(destinations))
	for i, destination := range destinations {
		before[i] = routeSource(destination)
	}
	var name string
	t.Run("create_and_configure", func(t *testing.T) {
		manager, err := client.New(configuration)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := manager.CloseTunnel(); err != nil {
				t.Error("close client tunnel:", err)
			}
		})
		// Exercise TunGo's real OS implementation without a server or handshake.
		if _, err := manager.OpenTunnel(netip.MustParseAddr("192.0.2.1")); err != nil {
			t.Fatal("open client tunnel:", err)
		}
		name = requireInterface(t, active)
		for _, destination := range destinations {
			want := active.IPv4.String()
			if netip.MustParseAddr(destination).Is6() {
				want = active.IPv6.String()
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				got := routeSource(destination)
				if got == want {
					t.Logf("route to %s selects TUN address %s", destination, got)
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("route to %s: source %s, want %s", destination, got, want)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	})
	requireRemoved(t, name)
	for i, destination := range destinations {
		if got := routeSource(destination); got != before[i] {
			t.Errorf("route to %s not restored: source %s, before %s", destination, got, before[i])
		}
	}
}

func TestLinuxServerNetwork(t *testing.T) {
	requireRunner(t)
	if runtime.GOOS != "linux" {
		t.Skip("TunGo server requires Linux")
	}
	for _, setting := range []string{"net.ipv4.ip_forward", "net.ipv6.conf.all.forwarding"} {
		before := command(t, "sysctl", "-n", setting)
		t.Cleanup(func() { command(t, "sysctl", "-w", setting+"="+before) })
		command(t, "sysctl", "-w", setting+"=0")
	}
	before := firewallState(t)
	configuration := settings.Settings{
		Network: settings.Network{
			TunName:    "s_probe0",
			IPv4Subnet: netip.MustParsePrefix("198.19.254.0/24"),
			IPv6Subnet: netip.MustParsePrefix("fd73:7467:6f:ff::/64"),
		},
		MTU: settings.DefaultMTU,
	}
	if err := configuration.DeriveIP(0); err != nil {
		t.Fatal(err)
	}
	var name string
	t.Run("create_and_configure", func(t *testing.T) {
		manager := server.NewManager()
		t.Cleanup(func() {
			if err := manager.CloseTunnel(configuration); err != nil {
				t.Error("close server tunnel:", err)
			}
		})
		tunnel, err := manager.OpenTunnel(configuration)
		if err != nil {
			t.Fatal("open server tunnel:", err)
		}
		t.Cleanup(func() {
			if err := tunnel.Close(); err != nil {
				t.Error("close server TUN descriptor:", err)
			}
		})
		name = requireInterface(t, configuration)
		for _, setting := range []string{"net.ipv4.ip_forward", "net.ipv6.conf.all.forwarding"} {
			if got := command(t, "sysctl", "-n", setting); got != "1" {
				t.Errorf("%s = %s, want 1", setting, got)
			}
		}
		for _, firewall := range []string{"iptables", "ip6tables"} {
			for _, table := range []string{"filter", "nat", "mangle"} {
				rules := command(t, firewall, "-t", table, "-S")
				match := configuration.TunName
				if table == "nat" {
					match = configuration.IPv4Subnet.String()
					if firewall == "ip6tables" {
						match = configuration.IPv6Subnet.String()
					}
				}
				if !strings.Contains(rules, match) {
					t.Errorf("%s %s has no TunGo rules: %s", firewall, table, rules)
				}
			}
		}
	})
	requireRemoved(t, name)
	if after := firewallState(t); !reflect.DeepEqual(after, before) {
		t.Errorf("firewall not restored:\nbefore=%v\nafter=%v", before, after)
	}
}

func requireRunner(t *testing.T) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("this probe changes host routes; use a disposable GitHub runner")
	}
	t.Logf("native %s/%s, process %d", runtime.GOOS, runtime.GOARCH, os.Getpid())
}

func routeSource(destination string) string {
	connection, err := net.DialTimeout("udp", net.JoinHostPort(destination, "9"), time.Second)
	if err != nil {
		return "unreachable"
	}
	defer func() { _ = connection.Close() }()
	return connection.LocalAddr().(*net.UDPAddr).IP.String()
}

func requireInterface(t *testing.T, configuration settings.Settings) string {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			t.Fatal(err)
		}
		found := map[netip.Addr]bool{}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err == nil {
				found[prefix.Addr().Unmap()] = true
			}
		}
		if found[configuration.IPv4] && found[configuration.IPv6] {
			if iface.MTU != configuration.MTU || iface.Flags&net.FlagUp == 0 {
				t.Fatalf("interface is not up with MTU %d: %+v", configuration.MTU, iface)
			}
			t.Logf("TUN %s is up, MTU %d, addresses %v", iface.Name, iface.MTU, addresses)
			return iface.Name
		}
	}
	t.Fatalf("TUN addresses missing: IPv4=%s, IPv6=%s", configuration.IPv4, configuration.IPv6)
	return ""
}

func requireRemoved(t *testing.T, name string) {
	t.Helper()
	if name == "" {
		return // Creation already failed; its cleanup still ran.
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := net.InterfaceByName(name); err != nil {
			t.Logf("TUN %s removed", name)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("TUN %s survived CloseTunnel", name)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func firewallState(t *testing.T) []string {
	t.Helper()
	var state []string
	for _, firewall := range []string{"iptables", "ip6tables"} {
		for _, table := range []string{"filter", "nat", "mangle"} {
			state = append(state, command(t, firewall, "-t", table, "-S"))
		}
	}
	return state
}

func command(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatal(fmt.Errorf("%v: %w: %s", args, err, output))
	}
	return strings.TrimSpace(string(output))
}
