//go:build windows

package networkchange

import (
	"context"
	"sync"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func Watch(ctx context.Context, tunIfIndex uint32) (<-chan struct{}, error) {
	changed := make(chan struct{})

	var once sync.Once
	signal := func() {
		once.Do(func() {
			close(changed)
		})
	}

	routeCB, err := winipcfg.RegisterRouteChangeCallback(
		func(_ winipcfg.MibNotificationType, row *winipcfg.MibIPforwardRow2) {
			if row.InterfaceIndex != tunIfIndex {
				signal()
			}
		},
	)
	if err != nil {
		return nil, err
	}

	interfaceCB, err := winipcfg.RegisterInterfaceChangeCallback(
		func(_ winipcfg.MibNotificationType, row *winipcfg.MibIPInterfaceRow) {
			if row.InterfaceIndex != tunIfIndex {
				signal()
			}
		},
	)
	if err != nil {
		_ = routeCB.Unregister()
		return nil, err
	}

	addressCB, err := winipcfg.RegisterUnicastAddressChangeCallback(
		func(_ winipcfg.MibNotificationType, row *winipcfg.MibUnicastIPAddressRow) {
			if row.InterfaceIndex != tunIfIndex {
				signal()
			}
		},
	)
	if err != nil {
		_ = interfaceCB.Unregister()
		_ = routeCB.Unregister()
		return nil, err
	}

	go func() {
		<-ctx.Done()

		_ = addressCB.Unregister()
		_ = interfaceCB.Unregister()
		_ = routeCB.Unregister()
	}()

	return changed, nil
}
