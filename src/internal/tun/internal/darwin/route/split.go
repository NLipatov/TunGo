//go:build darwin

package route

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func addSplitRoutes(dev string, splits []string) error {
	if len(splits) == 0 {
		return nil
	}
	iface, err := net.InterfaceByName(dev)
	if err != nil {
		return fmt.Errorf("find route interface %s: %w", dev, err)
	}
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return fmt.Errorf("open routing socket: %w", err)
	}
	unix.CloseOnExec(fd)
	defer func() { _ = unix.Close(fd) }()

	// Like route(8), discard notifications: RTM_ADD errors are returned by write.
	if err := unix.Shutdown(fd, unix.SHUT_RD); err != nil {
		return fmt.Errorf("disable routing socket reads: %w", err)
	}
	pid := uintptr(os.Getpid())
	for i, cidr := range splits {
		message, err := splitRouteMessage(cidr, iface.Index)
		if err != nil {
			return fmt.Errorf("route add %s: %w", cidr, err)
		}
		message.ID = pid
		message.Seq = i + 1
		data, err := message.Marshal()
		if err != nil {
			return fmt.Errorf("encode route %s: %w", cidr, err)
		}
		var n int
		for {
			n, err = unix.Write(fd, data)
			if !errors.Is(err, unix.EINTR) {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("route add %s: %w", cidr, err)
		}
		if n != len(data) {
			return fmt.Errorf("route add %s: %w", cidr, io.ErrShortWrite)
		}
	}
	return nil
}

func splitRouteMessage(cidr string, ifIndex int) (*route.RouteMessage, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}
	if prefix.Addr().Is4In6() {
		return nil, errors.New("IPv4-mapped IPv6 CIDR is not supported")
	}
	prefix = prefix.Masked()
	mask := net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen())
	message := &route.RouteMessage{
		Version: unix.RTM_VERSION,
		Type:    unix.RTM_ADD,
		Flags:   unix.RTF_UP | unix.RTF_STATIC,
		Addrs: []route.Addr{
			unix.RTAX_GATEWAY: &route.LinkAddr{Index: ifIndex},
			unix.RTAX_NETMASK: nil,
		},
	}
	if prefix.Addr().Is4() {
		message.Addrs[unix.RTAX_DST] = &route.Inet4Addr{IP: prefix.Addr().As4()}
		message.Addrs[unix.RTAX_NETMASK] = &route.Inet4Addr{IP: [4]byte(mask)}
	} else {
		message.Addrs[unix.RTAX_DST] = &route.Inet6Addr{IP: prefix.Addr().As16()}
		// Match route -inet6, which treats /128 as a host route without a mask.
		if prefix.Bits() == 128 {
			message.Flags |= unix.RTF_HOST
		} else {
			message.Addrs[unix.RTAX_NETMASK] = &route.Inet6Addr{IP: [16]byte(mask)}
		}
	}
	return message, nil
}
