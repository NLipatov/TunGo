//go:build darwin

package route

import (
	"bytes"
	"fmt"
	"net/netip"
	"strings"
	"tungo/infrastructure/PAL/exec_commander"

	"golang.org/x/sync/errgroup"
)

const (
	// v6SplitOne covers addresses between :: (0000:0000:0000:0000:0000:0000:0000:0000)
	// and 7fff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitOne = "::/1"
	// v6SplitTwo covers addresses between 8000:: (8000:0000:0000:0000:0000:0000:0000:0000)
	// and ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitTwo          = "8000::/1"
	loopbackIFaceNameV6 = "lo0"
	loopbackAddrV6      = "::1"
	linkLocalPrefixV6   = "fe80:"
)

type v6 struct {
	commander exec_commander.Commander
}

func newV6(commander exec_commander.Commander) Contract {
	return &v6{commander: commander}
}

func (v *v6) Get(destIP string) error {
	if ip, ipErr := netip.ParseAddr(destIP); ipErr != nil {
		return fmt.Errorf("v6.Get: invalid IP %q: %w", destIP, ipErr)
	} else if !ip.Is6() {
		return fmt.Errorf("v6.Get: non-IPv6 dest %q", destIP)
	} else if ip.IsLoopback() {
		return fmt.Errorf("v6.Get: invalid IP: loopback %q", destIP)
	}
	gateway, iFace, err := v.parseRoute(destIP)
	if err != nil {
		return err
	}
	// If route is empty or goes via loopback, try default route.
	if (gateway == "" && iFace == "") || v.isLoop(gateway, iFace) {
		if gwDef, ifDef, defErr := v.parseRoute("default"); defErr == nil {
			if gwDef != "" && !v.isLoop(gwDef, ifDef) {
				gateway, iFace = gwDef, ifDef
			}
		}
	}
	// If still loopback after fallback – treat as an error.
	if v.isLoop(gateway, iFace) {
		return fmt.Errorf("v6.Get: no non-loopback route found for destination: %q", destIP)
	}
	// Delete old route to destIP, ignore possible errors.
	_ = v.deleteQuiet(destIP)
	// For link-local gateways add interface scope if missing.
	if strings.HasPrefix(gateway, linkLocalPrefixV6) &&
		!strings.Contains(gateway, "%") &&
		iFace != "" {
		gateway = gateway + "%" + iFace
	}
	if gateway != "" && !strings.HasPrefix(gateway, "link#") {
		return v.addViaGatewayQuiet(destIP, gateway)
	}
	if iFace != "" {
		return v.addOnLinkQuiet(destIP, iFace)
	}
	return fmt.Errorf("no route found for %s", destIP)
}

func (v *v6) isLoop(gateway, iFace string) bool {
	return iFace == loopbackIFaceNameV6 || gateway == loopbackAddrV6
}

func (v *v6) parseRoute(target string) (gw, iFace string, err error) {
	out, err := v.commander.CombinedOutput("route", "-n", "-inet6", "get", target)
	if err != nil {
		return "", "", fmt.Errorf("route get %s: %w (%s)", target, err, out)
	}
	for _, ln := range strings.Split(string(out), "\n") {
		f := strings.Fields(strings.TrimSpace(ln))
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "gateway:":
			gw = f[1]
		case "interface:":
			iFace = f[1]
		}
	}
	return gw, iFace, nil
}

func (v *v6) Add(ip, iface string) error {
	_ = v.deleteQuiet(ip)
	return v.addOnLinkQuiet(ip, iface)
}

func (v *v6) AddViaGateway(ip, gw string) error {
	_ = v.deleteQuiet(ip)
	return v.addViaGatewayQuiet(ip, gw)
}

func (v *v6) Del(destIP string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "delete", "-inet6", destIP)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %s failed: %v (%s)", destIP, err, out)
	}
	return nil
}

func (v *v6) AddSplit(dev string) error {
	_ = v.runDeleteSplit("-inet6", v6SplitOne, "-interface", dev)
	_ = v.runDeleteSplit("-inet6", v6SplitTwo, "-interface", dev)

	if out, err := v.commander.CombinedOutput(
		"route", "-q", "-n", "add", "-inet6", v6SplitOne, "-interface", dev,
	); err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", v6SplitOne, err, out)
	}
	if out, err := v.commander.CombinedOutput(
		"route", "-q", "-n", "add", "-inet6", v6SplitTwo, "-interface", dev,
	); err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", v6SplitTwo, err, out)
	}
	return nil
}

func (v *v6) DelSplit(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error { return v.runDeleteSplit("-inet6", v6SplitOne, "-interface", dev) })
	eg.Go(func() error { return v.runDeleteSplit("-inet6", v6SplitTwo, "-interface", dev) })
	return eg.Wait()
}

func (v *v6) DefaultGateway() (string, error) {
	out, err := v.commander.CombinedOutput("route", "-n", "-inet6", "get", "default")
	if err != nil {
		return "", fmt.Errorf("defaultGateway(v6): %v (%s)", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "gateway:" {
			return f[1], nil
		}
	}
	return "", fmt.Errorf("defaultGateway(v6): no gateway found")
}

func (v *v6) deleteQuiet(ip string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "delete", "-inet6", ip)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %s failed: %v (%s)", ip, err, out)
	}
	return nil
}

func (v *v6) addOnLinkQuiet(ip, iface string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", ip, "-interface", iface)
	if err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s via interface %s failed: %v (%s)", ip, iface, err, out)
	}
	return nil
}

func (v *v6) addViaGatewayQuiet(ip, gw string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", ip, gw)
	if err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s via %s failed: %v (%s)", ip, gw, err, out)
	}
	return nil
}

func (v *v6) runDeleteSplit(args ...string) error {
	full := append([]string{"-q", "-n", "delete"}, args...)
	out, err := v.commander.CombinedOutput("route", full...)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %v failed: %v (%s)", args, err, out)
	}
	return nil
}
