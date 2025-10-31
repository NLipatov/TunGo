//go:build windows

package netsh

import (
	"fmt"
	"strconv"
	"tungo/infrastructure/PAL"
	"unicode"
)

// V6Wrapper is an IPv6 implementation of netsh.Contract.
type V6Wrapper struct {
	commander PAL.Commander
}

func NewV6Wrapper(commander PAL.Commander) Contract {
	return &V6Wrapper{commander: commander}
}

func (w *V6Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	for _, r := range mask {
		if !unicode.IsDigit(r) {
			return fmt.Errorf("SetAddressStatic: IPv6 requires prefix length, got mask=%q", mask)
		}
	}
	address := ip + "/" + mask
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "add", "address",
		`"`+ifName+`"`, address, "store=active",
	); err != nil {
		return fmt.Errorf("SetAddressStatic error: %v, output: %s", err, out)
	}
	return nil
}

func (w *V6Wrapper) SetAddressWithGateway(ifName, ip, mask, gateway string, metric int) error {
	if err := w.SetAddressStatic(ifName, ip, mask); err != nil {
		return err
	}
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "add", "route",
		"::/0",
		"interface="+`"`+ifName+`"`,
		"nexthop="+gateway,
		"metric="+strconv.Itoa(metric),
		"store=active",
	); err != nil {
		return fmt.Errorf("SetAddressWithGateway(add default route) error: %v, output: %s", err, out)
	}
	return nil
}

func (w *V6Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "address",
		"interface="+`"`+ifName+`"`,
		"address="+interfaceAddress,
	); err != nil {
		return fmt.Errorf("DeleteAddress error: %v, output: %s", err, out)
	}
	return nil
}

func (w *V6Wrapper) SetDNS(ifName string, dnsServers []string) error {
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "dnsservers",
		`"`+ifName+`"`, "all",
	)

	if len(dnsServers) == 0 {
		return nil
	}
	for i, dns := range dnsServers {
		index := i + 1
		if out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv6", "add", "dnsserver",
			`"`+ifName+`"`, dns, "index="+strconv.Itoa(index), "validate=no",
		); err != nil {
			return fmt.Errorf("SetDNS(add %s) error: %v, output: %s", dns, err, out)
		}
	}
	return nil
}

func (w *V6Wrapper) SetMTU(ifName string, mtu int) error {
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "set", "interface",
		`"`+ifName+`"`, "mtu="+strconv.Itoa(mtu), "store=active",
	); err != nil {
		return fmt.Errorf("SetMTU error: %v, output: %s", err, out)
	}
	return nil
}

func (w *V6Wrapper) AddRoutePrefix(prefix, ifName string, metric int) error {
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "add", "route",
		prefix,
		"interface="+`"`+ifName+`"`,
		"nexthop=::",
		"metric="+strconv.Itoa(metric),
		"store=active",
	); err != nil {
		return fmt.Errorf("AddRoutePrefix(%s) error: %v, output: %s", prefix, err, out)
	}
	return nil
}

func (w *V6Wrapper) DeleteRoutePrefix(prefix, ifName string) error {
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "route",
		prefix,
		"interface="+`"`+ifName+`"`,
		"nexthop=::",
	); err != nil {
		return fmt.Errorf("DeleteRoutePrefix(%s) error: %v, output: %s", prefix, err, out)
	}
	return nil
}

func (w *V6Wrapper) DeleteDefaultRoute(ifName string) error {
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "route",
		"::/0",
		"interface="+`"`+ifName+`"`,
	); err != nil {
		return fmt.Errorf("DeleteDefaultRoute error: %v, output: %s", err, out)
	}
	return nil
}

func (w *V6Wrapper) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	args := []string{
		"interface", "ipv6", "add", "route",
		hostIP + "/128",
		"interface=" + `"` + ifName + `"`,
		"nexthop=" + gateway,
		"metric=" + strconv.Itoa(metric),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteViaGateway(v6) error: %v, output: %s", err, out)
	}
	return nil
}

func (w *V6Wrapper) AddDefaultSplitRoutes(ifName string, metric int) error {
	halves := []string{"::/1", "8000::/1"}
	for _, p := range halves {
		out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv6", "add", "route",
			p, "interface="+`"`+ifName+`"`, "metric="+strconv.Itoa(metric), "store=active",
		)
		if err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v6 %s) error: %v, output: %s", p, err, out)
		}
	}
	return nil
}

func (w *V6Wrapper) DeleteDefaultSplitRoutes(ifName string) error {
	halves := []string{"::/1", "8000::/1"}
	var last error
	for _, p := range halves {
		if out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv6", "delete", "route",
			p, "interface="+`"`+ifName+`"`,
		); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(v6 %s) error: %v, output: %s", p, err, out)
		}
	}
	return last
}
