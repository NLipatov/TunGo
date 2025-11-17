//go:build darwin

package route

import (
	"bytes"
	"fmt"
	"golang.org/x/sync/errgroup"
	"net/netip"
	"strings"
	"tungo/infrastructure/PAL"
)

const (
	// v4SplitOne covers half of IPv4 address space
	// (addresses between 0.0.0.0 and 127.255.255.255)
	v4SplitOne = "0.0.0.0/1"
	// v4SplitTwo covers half of IPv4 address space
	// (addresses between 128.0.0.0 and 255.255.255.255)
	v4SplitTwo        = "128.0.0.0/1"
	loopbackIFaceName = "lo0"
	loopbackPrefix    = "127."
)

type v4 struct {
	commander PAL.Commander
}

func newV4(commander PAL.Commander) Contract {
	return &v4{
		commander: commander,
	}
}

func (v *v4) Get(destIP string) error {
	if ip, ipErr := netip.ParseAddr(destIP); ipErr != nil {
		return fmt.Errorf("v4.Get: invalid IP %q: %w", destIP, ipErr)
	} else if !ip.Is4() {
		return fmt.Errorf("v4.Get: non-IPv4 dest %q", destIP)
	} else if ip.IsLoopback() {
		return fmt.Errorf("v4.Get: invalid IP: loopback %q", destIP)
	}
	gateway, iFace, err := v.parseRoute(destIP)
	if err != nil {
		return err
	}
	if (gateway == "" && iFace == "") || v.isLoop(gateway, iFace) {
		if gwDef, ifDef, defErr := v.parseRoute("default"); defErr == nil {
			if gwDef != "" && !strings.HasPrefix(gwDef, loopbackPrefix) {
				gateway, iFace = gwDef, ifDef
			}
		}
	}
	if v.isLoop(gateway, iFace) {
		return fmt.Errorf("v4.Get: no non-loopback route found for destination: %q", destIP)
	}
	_ = v.deleteQuiet(destIP)
	if gateway != "" && !strings.HasPrefix(gateway, "link#") {
		return v.addViaGatewayQuiet(destIP, gateway)
	}
	if iFace != "" {
		return v.addOnLinkQuiet(destIP, iFace)
	}
	return fmt.Errorf("no route found for %s", destIP)
}

func (v *v4) isLoop(gateway, iFace string) bool {
	return iFace == loopbackIFaceName || strings.HasPrefix(gateway, loopbackPrefix)
}

func (v *v4) parseRoute(target string) (gw, iFace string, err error) {
	out, err := v.commander.CombinedOutput("route", "-n", "get", target)
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

func (v *v4) Add(ip, iFace string) error {
	_ = v.deleteQuiet(ip)
	return v.addOnLinkQuiet(ip, iFace)
}

func (v *v4) AddViaGateway(ip, gw string) error {
	_ = v.deleteQuiet(ip)
	return v.addViaGatewayQuiet(ip, gw)
}

func (v *v4) Del(destIP string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "delete", destIP)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %s failed: %v (%s)", destIP, err, out)
	}
	return nil
}

func (v *v4) AddSplit(dev string) error {
	_ = v.runDeleteSplit("-net", v4SplitOne, "-interface", dev)
	_ = v.runDeleteSplit("-net", v4SplitTwo, "-interface", dev)

	if out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-net", v4SplitOne, "-interface", dev); err != nil &&
		!bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", v4SplitOne, err, out)
	}
	if out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-net", v4SplitTwo, "-interface", dev); err != nil &&
		!bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", v4SplitTwo, err, out)
	}
	return nil
}

func (v *v4) DelSplit(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error { return v.runDeleteSplit("-net", v4SplitOne, "-interface", dev) })
	eg.Go(func() error { return v.runDeleteSplit("-net", v4SplitTwo, "-interface", dev) })
	return eg.Wait()
}

func (v *v4) DefaultGateway() (string, error) {
	out, err := v.commander.CombinedOutput("route", "-n", "get", "default")
	if err != nil {
		return "", fmt.Errorf("defaultGateway: %v (%s)", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "gateway:" {
			return f[1], nil
		}
	}
	return "", fmt.Errorf("defaultGateway: no gateway found")
}

func (v *v4) deleteQuiet(ip string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "delete", ip)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %s failed: %v (%s)", ip, err, out)
	}
	return nil
}

func (v *v4) addOnLinkQuiet(ip, iFace string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", ip, "-interface", iFace)
	if err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s via interface %s failed: %v (%s)", ip, iFace, err, out)
	}
	return nil
}

func (v *v4) addViaGatewayQuiet(ip, gw string) error {
	out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", ip, gw)
	if err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s via %s failed: %v (%s)", ip, gw, err, out)
	}
	return nil
}

func (v *v4) runDeleteSplit(args ...string) error {
	full := append([]string{"-q", "-n", "delete"}, args...)
	out, err := v.commander.CombinedOutput("route", full...)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %v failed: %v (%s)", args, err, out)
	}
	return nil
}
