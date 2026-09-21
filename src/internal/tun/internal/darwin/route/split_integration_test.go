//go:build darwin

package route

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"tungo/internal/tun/internal/darwin/utun"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func TestAddSplitRoutesKernel(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("routing socket integration test requires root")
	}
	for _, tt := range []struct {
		name   string
		family int
		splits []string
	}{
		{"IPv4", unix.AF_INET, []string{"198.51.100.0/25", "203.0.113.17/32", "198.51.100.128/26", "203.0.113.64/26"}},
		{"IPv6", unix.AF_INET6, []string{"2001:db8:1::/65", "2001:db8:2::17/128", "2001:db8:3::/64", "2001:db8:4::/64"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for prefix := range kernelRoutes(t, tt.family, 0) {
				for _, cidr := range tt.splits {
					if prefix == netip.MustParsePrefix(cidr) {
						t.Skipf("preserving existing route %s", cidr)
					}
				}
			}
			tun, err := utun.New()
			if err != nil {
				t.Fatalf("create temporary utun: %v", err)
			}
			t.Cleanup(func() {
				if err := tun.Close(); err != nil {
					t.Errorf("close temporary utun: %v", err)
				}
			})
			iface, err := net.InterfaceByName(tun.Name())
			if err != nil {
				t.Fatal(err)
			}
			args := []string{tun.Name(), "inet", "192.0.2.1", "192.0.2.1", "netmask", "255.255.255.255", "up"}
			if tt.family == unix.AF_INET6 {
				args = []string{tun.Name(), "inet6", "2001:db8:ffff::1", "prefixlen", "128", "up"}
			}
			if out, err := exec.Command("/sbin/ifconfig", args...).CombinedOutput(); err != nil {
				t.Fatalf("configure temporary utun: %v (%s)", err, out)
			}

			cmd := newMockRunner()
			add := NewV4(cmd).AddSplit
			if tt.family == unix.AF_INET6 {
				add = NewV6(cmd).AddSplit
			}
			if err := add(tun.Name(), tt.splits[:2]); err != nil {
				t.Fatalf("add split routes: %v", err)
			}
			if len(cmd.allCalls()) != 0 {
				t.Fatalf("split routes invoked external commands: %v", cmd.allCalls())
			}
			installed := kernelRoutes(t, tt.family, iface.Index)
			for _, cidr := range tt.splits[:2] {
				if _, ok := installed[netip.MustParsePrefix(cidr)]; !ok {
					t.Fatalf("route %s not installed on %s: %v", cidr, tun.Name(), installed)
				}
			}

			// A duplicate must fail without deleting the existing route or adding later entries.
			err = add(tun.Name(), []string{tt.splits[2], tt.splits[0], tt.splits[3]})
			if !errors.Is(err, unix.EEXIST) || !strings.Contains(err.Error(), tt.splits[0]) {
				t.Fatalf("duplicate route error = %v, want EEXIST with CIDR", err)
			}
			installed = kernelRoutes(t, tt.family, iface.Index)
			for i, cidr := range tt.splits {
				_, exists := installed[netip.MustParsePrefix(cidr)]
				if exists != (i < 3) {
					t.Fatalf("route %s present = %v after failed add: %v", cidr, exists, installed)
				}
			}

			if err := tun.Close(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for len(kernelRoutes(t, tt.family, iface.Index)) != 0 {
				if time.Now().After(deadline) {
					t.Fatal("routes remain after closing temporary utun")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func kernelRoutes(t *testing.T, family, ifIndex int) map[netip.Prefix]int {
	t.Helper()
	rib, err := route.FetchRIB(family, route.RIBTypeRoute, 0)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		t.Fatal(err)
	}
	routes := make(map[netip.Prefix]int)
	for _, message := range messages {
		entry, ok := message.(*route.RouteMessage)
		if !ok || (ifIndex != 0 && entry.Index != ifIndex) {
			continue
		}
		destination := routeAddress(entry.Addrs[unix.RTAX_DST])
		if !destination.IsValid() {
			continue
		}
		bits := 0
		if entry.Flags&unix.RTF_HOST != 0 {
			bits = destination.BitLen()
		} else if mask := routeAddress(entry.Addrs[unix.RTAX_NETMASK]); mask.IsValid() {
			var size int
			bits, size = net.IPMask(mask.AsSlice()).Size()
			if size != destination.BitLen() {
				t.Fatalf("unexpected route mask %s for %s", mask, destination)
			}
		}
		routes[netip.PrefixFrom(destination, bits).Masked()] = entry.Flags
	}
	return routes
}
