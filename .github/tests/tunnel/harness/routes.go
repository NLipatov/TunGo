package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"strings"
)

// runnerRoutes preserves control traffic outside the test networks and owns
// the exact commands needed to remove only the routes it successfully added.
type runnerRoutes struct {
	undo [][]string
}

func (r *runnerRoutes) install(ctx context.Context) error {
	var gateway, iface string
	switch runtime.GOOS {
	case "linux":
		rows, err := routeRows(ctx, "ip", "-j", "-4", "route", "get", "1.1.1.1")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("runner has no IPv4 default route")
		}
		gateway, _ = rows[0]["gateway"].(string)
		iface, _ = rows[0]["dev"].(string)
	case "darwin":
		output, err := command(ctx, "route", "-n", "get", "default")
		if err != nil {
			return err
		}
		gateway, err = routeField(output, "gateway")
		if err != nil {
			return err
		}
	case "windows":
		output, err := powershell(ctx, "Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Sort-Object RouteMetric | Select-Object -First 1 -Property NextHop,InterfaceIndex | ConvertTo-Json -Compress")
		if err != nil {
			return err
		}
		var route struct {
			NextHop        string
			InterfaceIndex int
		}
		if err := json.Unmarshal([]byte(output), &route); err != nil {
			return err
		}
		gateway, iface = route.NextHop, fmt.Sprint(route.InterfaceIndex)
	default:
		return fmt.Errorf("unsupported runner OS %s", runtime.GOOS)
	}
	if gateway == "" {
		return fmt.Errorf("runner IPv4 route has no gateway")
	}
	for _, prefix := range preservedPrefixes(netip.MustParsePrefix("198.18.0.0/15")) {
		if err := r.add(ctx, prefix.String(), gateway, iface); err != nil {
			return err
		}
	}
	// Only preserve public IPv6 if it already has a gateway. Test ULAs remain
	// exclusively routed by TunGo even when the runner has no IPv6 default route.
	switch runtime.GOOS {
	case "linux":
		rows, err := routeRows(ctx, "ip", "-j", "-6", "route", "show", "default")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		route := rows[0]
		for _, row := range rows[1:] {
			metric, _ := row["metric"].(float64)
			best, _ := route["metric"].(float64)
			if metric < best {
				route = row
			}
		}
		gateway, _ = route["gateway"].(string)
		iface, _ = route["dev"].(string)
	case "darwin":
		output, err := command(ctx, "route", "-n", "get", "-inet6", "default")
		if err != nil {
			return nil
		}
		gateway, err = routeField(output, "gateway")
		if err != nil {
			return nil
		}
	case "windows":
		output, err := powershell(ctx, "Get-NetRoute -AddressFamily IPv6 | Where-Object DestinationPrefix -eq '::/0' | Sort-Object RouteMetric | Select-Object -First 1 -Property NextHop,InterfaceIndex | ConvertTo-Json -Compress")
		if err != nil {
			return err
		}
		if output == "" {
			return nil
		}
		var route struct {
			NextHop        string
			InterfaceIndex int
		}
		if err := json.Unmarshal([]byte(output), &route); err != nil {
			return err
		}
		gateway, iface = route.NextHop, fmt.Sprint(route.InterfaceIndex)
	}
	return r.add(ctx, "2000::/3", gateway, iface)
}

func preservedPrefixes(excluded netip.Prefix) []netip.Prefix {
	var result []netip.Prefix
	for bit := 0; bit < excluded.Bits(); bit++ {
		address := excluded.Addr().As4()
		address[bit/8] ^= 1 << (7 - bit%8)
		prefix := netip.PrefixFrom(netip.AddrFrom4(address), bit+1).Masked()
		if prefix.Bits() == 1 {
			// Both /1 routes belong exclusively to TunGo.
			address = prefix.Addr().As4()
			result = append(result, netip.PrefixFrom(netip.AddrFrom4(address), 2))
			address[0] |= 64
			result = append(result, netip.PrefixFrom(netip.AddrFrom4(address), 2))
		} else {
			result = append(result, prefix)
		}
	}
	return result
}

func (r *runnerRoutes) add(ctx context.Context, prefix, gateway, iface string) error {
	var add, remove []string
	switch runtime.GOOS {
	case "linux":
		family := "-4"
		if strings.Contains(prefix, ":") {
			family = "-6"
		}
		suffix := []string{prefix, "via", gateway, "dev", iface}
		add = append([]string{"ip", family, "route", "add"}, suffix...)
		remove = append([]string{"ip", family, "route", "del"}, suffix...)
	case "darwin":
		family := "-net"
		if strings.Contains(prefix, ":") {
			family = "-inet6"
		}
		add = []string{"route", "-n", "add", family, prefix, gateway}
		remove = []string{"route", "-n", "delete", family, prefix, gateway}
	case "windows":
		args := []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command"}
		add = append(append([]string{}, args...), fmt.Sprintf("$ErrorActionPreference='Stop'; New-NetRoute -DestinationPrefix '%s' -NextHop '%s' -InterfaceIndex %s -PolicyStore ActiveStore | Out-Null", prefix, gateway, iface))
		remove = append(append([]string{}, args...), fmt.Sprintf("$ErrorActionPreference='Stop'; Get-NetRoute -DestinationPrefix '%s' -NextHop '%s' -InterfaceIndex %s -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false", prefix, gateway, iface))
	}
	if _, err := command(ctx, add...); err != nil {
		return err
	}
	r.undo = append(r.undo, remove)
	return nil
}

func (r *runnerRoutes) remove(ctx context.Context) error {
	var errs []error
	for i := len(r.undo) - 1; i >= 0; i-- {
		_, err := command(ctx, r.undo[i]...)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
