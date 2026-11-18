//go:build windows

package defaultroute

import (
	"context"
	"net/netip"
	"sync"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func Watch(ctx context.Context, serverAddr netip.Addr) (<-chan error, error) {
	changed := make(chan struct{})
	errCh := make(chan error, 1)

	family := addrFamily(serverAddr)
	var once sync.Once
	callback, err := winipcfg.RegisterRouteChangeCallback(
		func(_ winipcfg.MibNotificationType, route *winipcfg.MibIPforwardRow2) {
			if isDefaultRouteChange(route, family) {
				once.Do(func() {
					close(changed)
				})
			}
		},
	)
	if err != nil {
		return nil, err
	}

	go func() {
		select {
		case <-ctx.Done():
			_ = callback.Unregister()
		case <-changed:
			errCh <- callback.Unregister()
		}
	}()

	return errCh, nil
}

func addrFamily(addr netip.Addr) winipcfg.AddressFamily {
	if addr.Unmap().Is4() {
		return windows.AF_INET
	}
	return windows.AF_INET6
}

func isDefaultRouteChange(route *winipcfg.MibIPforwardRow2, family winipcfg.AddressFamily) bool {
	if route == nil || route.DestinationPrefix.RawPrefix.Family != family {
		return false
	}
	prefix := route.DestinationPrefix.Prefix()
	return prefix.IsValid() && prefix.Bits() == 0
}
