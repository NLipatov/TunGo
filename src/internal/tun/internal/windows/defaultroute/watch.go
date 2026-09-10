//go:build windows

package defaultroute

import (
	"context"
	"sync"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func Watch(ctx context.Context) (<-chan struct{}, error) {
	changed := make(chan struct{})

	var once sync.Once
	callback, err := winipcfg.RegisterRouteChangeCallback(
		func(_ winipcfg.MibNotificationType, route *winipcfg.MibIPforwardRow2) {
			if isDefaultRouteChange(route) {
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
		case <-changed:
		}
		_ = callback.Unregister()
	}()

	return changed, nil
}

func isDefaultRouteChange(route *winipcfg.MibIPforwardRow2) bool {
	if route == nil {
		return false
	}
	prefix := route.DestinationPrefix.Prefix()
	return prefix.IsValid() && prefix.Bits() == 0
}
