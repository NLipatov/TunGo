//go:build linux

package defaultroute

import (
	"context"
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func Watch(ctx context.Context) (<-chan struct{}, error) {
	fd, err := unix.Socket(
		unix.AF_NETLINK,
		unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK,
		unix.NETLINK_ROUTE,
	)
	if err != nil {
		return nil, err
	}

	groups := uint32(unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: groups,
	}); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	changed := make(chan struct{})
	go func() {
		defer func() {
			_ = unix.Close(fd)
		}()

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
				if isDefaultRouteChange(message) {
					close(changed)
					return
				}
			}
		}
	}()

	return changed, nil
}

func isDefaultRouteChange(message syscall.NetlinkMessage) bool {
	const (
		destinationPrefixLengthOffset = 1
		routingTableOffset            = 4
	)
	return (message.Header.Type == unix.RTM_NEWROUTE ||
		message.Header.Type == unix.RTM_DELROUTE) &&
		len(message.Data) >= unix.SizeofRtMsg &&
		message.Data[destinationPrefixLengthOffset] == 0 &&
		message.Data[routingTableOffset] == unix.RT_TABLE_MAIN
}
