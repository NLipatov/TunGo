//go:build darwin

package networkchange

import (
	"context"
	"errors"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func Watch(ctx context.Context, tunIfIndex int) (<-chan struct{}, error) {
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return nil, err
	}

	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
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

			n, err = unix.Read(fd, buffer)
			if errors.Is(err, unix.EAGAIN) {
				continue
			}
			if err != nil {
				return
			}

			messages, err := route.ParseRIB(0, buffer[:n])
			if err != nil {
				continue
			}

			for _, message := range messages {
				switch m := message.(type) {
				case *route.RouteMessage:
					switch m.Type {
					case unix.RTM_ADD, unix.RTM_DELETE, unix.RTM_CHANGE:
						if m.Index != tunIfIndex {
							close(changed)
							return
						}
					}

				case *route.InterfaceMessage:
					if m.Index != tunIfIndex {
						close(changed)
						return
					}

				case *route.InterfaceAddrMessage:
					if m.Index != tunIfIndex {
						close(changed)
						return
					}
				}
			}
		}
	}()

	return changed, nil
}
