//go:build linux

package networkchange

import (
	"context"
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func Watch(ctx context.Context, tunIfIndex int) (<-chan struct{}, error) {
	fd, err := unix.Socket(
		unix.AF_NETLINK,
		unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK,
		unix.NETLINK_ROUTE,
	)
	if err != nil {
		return nil, err
	}

	groups := uint32(
		unix.RTMGRP_LINK |
			unix.RTMGRP_IPV4_IFADDR |
			unix.RTMGRP_IPV6_IFADDR |
			unix.RTMGRP_IPV4_ROUTE |
			unix.RTMGRP_IPV6_ROUTE,
	)

	err = unix.Bind(fd, &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: groups,
	})
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	changed := make(chan struct{})

	go func() {
		defer unix.Close(fd)

		poll := []unix.PollFd{{
			Fd:     int32(fd),
			Events: unix.POLLIN,
		}}
		buffer := make([]byte, 64*1024)

		for ctx.Err() == nil {
			n, err := unix.Poll(poll, 500)
			if errors.Is(err, unix.EINTR) || n == 0 {
				continue
			}
			if err != nil {
				return
			}

			n, _, err = unix.Recvfrom(fd, buffer, 0)
			if errors.Is(err, unix.EAGAIN) {
				continue
			}
			if err != nil {
				return
			}

			messages, err := syscall.ParseNetlinkMessage(buffer[:n])
			if err != nil {
				continue
			}

			for _, message := range messages {
				switch message.Header.Type {
				case unix.RTM_NEWLINK, unix.RTM_DELLINK,
					unix.RTM_NEWADDR, unix.RTM_DELADDR,
					unix.RTM_NEWROUTE, unix.RTM_DELROUTE:

					// Здесь следует разобрать payload и отбросить
					// события собственного TUN по tunIfIndex.
					close(changed)
					return
				}
			}
		}
	}()

	return changed, nil
}
