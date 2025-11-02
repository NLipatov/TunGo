package ipcfg

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"net"
	"net/netip"
	"strings"
)

func displayNameFromLUID(luid winipcfg.LUID, ifIndex uint32) string {
	if ifRow, _ := luid.Interface(); ifRow != nil {
		if s := strings.TrimSpace(ifRow.Alias()); s != "" {
			return s
		}
		if s := strings.TrimSpace(ifRow.Description()); s != "" {
			return s
		}
	}
	if addrs, err := winipcfg.GetAdaptersAddresses(winipcfg.AddressFamily(windows.AF_UNSPEC), 0); err == nil {
		for _, a := range addrs {
			if a.LUID == luid {
				if s := strings.TrimSpace(a.FriendlyName()); s != "" {
					return s
				}
				break
			}
		}
	}
	if ifIndex != 0 {
		return fmt.Sprintf("if#%d", ifIndex)
	}
	return ""
}

// luidByName resolves interface LUID by FriendlyName (as shown in Windows UI).
func luidByName(ifName string) (winipcfg.LUID, error) {
	addrs, err := winipcfg.GetAdaptersAddresses(winipcfg.AddressFamily(windows.AF_UNSPEC), 0)
	if err != nil {
		return 0, err
	}
	for _, a := range addrs {
		if a.FriendlyName() == ifName {
			return a.LUID, nil
		}
	}
	return 0, fmt.Errorf("interface %q not found", ifName)
}

func dottedMaskToPrefix(ipStr, maskStr string) (netip.Prefix, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return netip.Prefix{}, fmt.Errorf("ip is not IPv4: %q", ipStr)
	}
	m := net.ParseIP(maskStr)
	if m == nil || m.To4() == nil {
		return netip.Prefix{}, fmt.Errorf("mask is not dotted IPv4: %q", maskStr)
	}
	ones, bits := net.IPMask(m.To4()).Size()
	if bits != 32 || ones < 0 || ones > 32 {
		return netip.Prefix{}, fmt.Errorf("bad mask: %q", maskStr)
	}
	addr, addrErr := netip.ParseAddr(ip.To4().String())
	if addrErr != nil {
		return netip.Prefix{}, fmt.Errorf("parse addr failed: %q", ipStr)
	}
	return netip.PrefixFrom(addr, ones), nil
}

func parseIPv4Prefix(cidr string) (netip.Prefix, error) {
	pfx, pfxErr := netip.ParsePrefix(strings.TrimSpace(cidr))
	if pfxErr != nil || !pfx.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("bad IPv4 prefix: %q", cidr)
	}
	return pfx, nil
}
