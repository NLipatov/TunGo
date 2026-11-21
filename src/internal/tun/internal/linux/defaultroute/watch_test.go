//go:build linux

package defaultroute

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBuildRouteRequest(t *testing.T) {
	for _, server := range []netip.Addr{netip.MustParseAddr("198.51.100.7"), netip.MustParseAddr("2001:db8::7")} {
		t.Run(server.String(), func(t *testing.T) {
			messages, err := syscall.ParseNetlinkMessage(buildRouteRequest(server))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(messages) != 1 {
				t.Fatalf("got %d messages, want 1", len(messages))
			}
			m := messages[0]
			if m.Header.Type != unix.RTM_GETROUTE || m.Header.Flags != unix.NLM_F_REQUEST {
				t.Fatalf("got type %d flags %d, want a route lookup request", m.Header.Type, m.Header.Flags)
			}
			if int(m.Data[familyOffset]) != addrFamily(server) || int(m.Data[destinationPrefixLengthOffset]) != server.BitLen() {
				t.Fatalf("got family %d prefix length %d", m.Data[familyOffset], m.Data[destinationPrefixLengthOffset])
			}
			// The standard parser reads attributes of route notifications only.
			m.Header.Type = unix.RTM_NEWROUTE
			attrs, err := syscall.ParseNetlinkRouteAttr(&m)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(attrs) != 1 || attrs[0].Attr.Type != unix.RTA_DST || !bytes.Equal(attrs[0].Value, server.AsSlice()) {
				t.Fatalf("got attributes %+v, want the destination %v", attrs, server)
			}
		})
	}
}

func TestRouteReply(t *testing.T) {
	server := netip.MustParseAddr("198.51.100.7")
	gateway := netip.MustParseAddr("10.0.0.1")
	errno := -int32(unix.ENETUNREACH)
	errorReply := syscall.NetlinkMessage{
		Header: syscall.NlMsghdr{Type: unix.NLMSG_ERROR},
		Data:   binary.NativeEndian.AppendUint32(nil, uint32(errno)),
	}

	t.Run("route", func(t *testing.T) {
		got, err := routeReply([]syscall.NetlinkMessage{
			routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
		})
		want := route{gateway: gateway, oif: 2}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v, want %+v", got, err, want)
		}
	})
	t.Run("kernel error", func(t *testing.T) {
		if _, err := routeReply([]syscall.NetlinkMessage{errorReply}); !errors.Is(err, unix.ENETUNREACH) {
			t.Fatalf("got %v, want %v", err, unix.ENETUNREACH)
		}
	})
	t.Run("short error", func(t *testing.T) {
		short := syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: unix.NLMSG_ERROR}}
		if _, err := routeReply([]syscall.NetlinkMessage{short}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("route from a policy routing table", func(t *testing.T) {
		got, err := routeReply([]syscall.NetlinkMessage{
			routeMessage(unix.RTM_NEWROUTE, 32, 100, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
		})
		want := route{gateway: gateway, oif: 2}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v, want %+v", got, err, want)
		}
	})
	t.Run("local route", func(t *testing.T) {
		got, err := routeReply([]syscall.NetlinkMessage{
			routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_LOCAL, unix.RTN_LOCAL, dstAttr(server), oifAttr(1)),
		})
		want := route{oif: 1}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v, want %+v", got, err, want)
		}
	})
	t.Run("route without interface", func(t *testing.T) {
		reply := routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway))
		if _, err := routeReply([]syscall.NetlinkMessage{reply}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("unexpected message", func(t *testing.T) {
		reply := syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: unix.RTM_NEWLINK}}
		if _, err := routeReply([]syscall.NetlinkMessage{reply}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("unreachable route", func(t *testing.T) {
		reply := routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNREACHABLE, dstAttr(server))
		if _, err := routeReply([]syscall.NetlinkMessage{reply}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := routeReply(nil); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
}

func TestLookupRouteAsksTheKernel(t *testing.T) {
	_, err := lookup(netip.MustParseAddr("192.0.2.1"))
	// A host without a route to the address gets the kernel's error instead.
	if err != nil && !errors.Is(err, unix.ENETUNREACH) {
		t.Fatalf("got %v, want a route or %v", err, unix.ENETUNREACH)
	}
}

func TestLookupRouteToLoopback(t *testing.T) {
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := lookup(netip.MustParseAddr("127.0.0.1"))
	if err != nil || got.oif != uint32(lo.Index) || got.gateway.IsValid() {
		t.Fatalf("got %+v, %v, want a route through interface %d", got, err, lo.Index)
	}
}

func TestWatchKeepsAnUnchangedRoute(t *testing.T) {
	// Through a local proxy the client sees the proxy's loopback address as the server.
	errCh, err := Watch(t.Context(), netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("got %v, want no signal", err)
	case <-time.After(time.Second): // two checks
	}
}

func TestWatchKernel(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("route watcher integration test requires root")
	}
	const (
		link   = "tungo-wt0"
		server = "203.0.113.7"
	)
	if out, err := exec.Command("ip", "-4", "route", "show", server+"/32").CombinedOutput(); err != nil || len(bytes.TrimSpace(out)) != 0 {
		t.Skipf("preserving existing route to %s: %s %v", server, out, err)
	}
	ip(t, "link", "add", link, "type", "dummy")
	t.Cleanup(func() {
		_ = exec.Command("ip", "route", "del", server+"/32").Run()
		_ = exec.Command("ip", "link", "del", link).Run()
	})
	ip(t, "addr", "add", "192.0.2.1/24", "dev", link)
	ip(t, "link", "set", link, "up")
	serverAddr := netip.MustParseAddr(server)

	t.Run("another gateway", func(t *testing.T) {
		ip(t, "route", "replace", server, "via", "192.0.2.2", "dev", link)
		errCh, err := Watch(t.Context(), serverAddr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ip(t, "route", "replace", server, "via", "192.0.2.3", "dev", link)
		if err := signal(t, errCh); err != nil {
			t.Fatalf("got %v, want nil", err)
		}
	})
	t.Run("no route", func(t *testing.T) {
		ip(t, "route", "replace", server, "via", "192.0.2.2", "dev", link)
		errCh, err := Watch(t.Context(), serverAddr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ip(t, "route", "replace", "unreachable", server)
		if err := signal(t, errCh); !errors.Is(err, unix.EHOSTUNREACH) {
			t.Fatalf("got %v, want %v", err, unix.EHOSTUNREACH)
		}
	})
	t.Run("no route at start", func(t *testing.T) {
		ip(t, "route", "replace", "unreachable", server)
		if _, err := Watch(t.Context(), serverAddr); !errors.Is(err, unix.EHOSTUNREACH) {
			t.Fatalf("got %v, want %v", err, unix.EHOSTUNREACH)
		}
	})
}

func ip(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
		t.Fatalf("ip %v: %v %s", args, err, out)
	}
}

// signal waits for the watcher to report, allowing a few lookup intervals.
func signal(t *testing.T, errCh <-chan error) error {
	t.Helper()
	select {
	case err := <-errCh:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("got no signal")
		return nil
	}
}

func lookup(addr netip.Addr) (route, error) {
	return lookupRoute(make([]byte, unix.Getpagesize()), buildRouteRequest(addr), &unix.SockaddrNetlink{Family: unix.AF_NETLINK})
}

// routingTableOffset is the offset of rtm_table in struct rtmsg.
const routingTableOffset = 4

type routeAttr struct {
	kind  uint16
	value []byte
}

func dstAttr(addr netip.Addr) routeAttr {
	return routeAttr{kind: unix.RTA_DST, value: addr.AsSlice()}
}

func gatewayAttr(addr netip.Addr) routeAttr {
	return routeAttr{kind: unix.RTA_GATEWAY, value: addr.AsSlice()}
}

func oifAttr(index uint32) routeAttr {
	return routeAttr{kind: unix.RTA_OIF, value: binary.NativeEndian.AppendUint32(nil, index)}
}

func routeMessage(kind uint16, dstLen, table, routeType byte, attrs ...routeAttr) syscall.NetlinkMessage {
	data := make([]byte, unix.SizeofRtMsg)
	data[destinationPrefixLengthOffset] = dstLen
	data[routingTableOffset] = table
	data[routeTypeOffset] = routeType
	for _, attr := range attrs {
		length := unix.SizeofRtAttr + len(attr.value)
		data = binary.NativeEndian.AppendUint16(data, uint16(length))
		data = binary.NativeEndian.AppendUint16(data, attr.kind)
		data = append(data, attr.value...)
		aligned := (length + unix.RTA_ALIGNTO - 1) &^ (unix.RTA_ALIGNTO - 1)
		data = append(data, make([]byte, aligned-length)...)
	}
	return syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: kind}, Data: data}
}
