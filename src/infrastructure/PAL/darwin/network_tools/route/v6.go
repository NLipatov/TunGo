//go:build darwin

package route

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"golang.org/x/sync/errgroup"
	"tungo/infrastructure/PAL"
)

const (
	// v6SplitOne covers addresses between :: (0000:0000:0000:0000:0000:0000:0000:0000) and 7fff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitOne = "::/1"
	// v6SplitTwo covers addresses between 8000:: (8000:0000:0000:0000:0000:0000:0000:0000) and ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitTwo = "8000::/1"
)

type v6 struct {
	commander PAL.Commander
}

func newV6(commander PAL.Commander) Contract {
	return &v6{commander: commander}
}

func (v *v6) Get(destIP string) error {
	ip := net.ParseIP(destIP)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("v6.Get: non-IPv6 dest %q", destIP)
	} else if ip.IsLoopback() {
		return fmt.Errorf("v6.Get: invalid IP: loopbackßßß %q", destIP)
	}
	gw, iFace, err := v.parseRoute(destIP)
	if err != nil {
		return err
	}
	isLoop := gw == "::1"
	if (gw == "" && iFace == "") || isLoop {
		if gwDef, ifDef, err2 := v.parseRoute("default"); err2 == nil {
			if gwDef != "" && gwDef != "::1" {
				gw, iFace = gwDef, ifDef
			}
		}
	}
	_ = v.deleteQuiet(destIP)
	if strings.HasPrefix(gw, "fe80:") && !strings.Contains(gw, "%") && iFace != "" {
		gw = gw + "%" + iFace
	}

	switch {
	case gw != "" && !strings.HasPrefix(gw, "link#"):
		return v.addViaGatewayQuiet(destIP, gw)
	case iFace != "":
		return v.addOnLinkQuiet(destIP, iFace)
	default:
		return fmt.Errorf("no route found for %s", destIP)
	}
}

func (v *v6) parseRoute(target string) (gw, iface string, err error) {
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
			iface = f[1]
		}
	}
	return gw, iface, nil
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

	if out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", v6SplitOne, "-interface", dev); err != nil &&
		!bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", v6SplitOne, err, out)
	}
	if out, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", v6SplitTwo, "-interface", dev); err != nil &&
		!bytes.Contains(out, []byte("File exists")) {
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
		return "", fmt.Errorf("defaultGateway(v6): %v", err)
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

func (v *v6) runDeleteSplit(protoFlag string, cidr string, extra ...string) error {
	args := append([]string{"-q", "-n", "delete", protoFlag, cidr}, extra...)
	out, err := v.commander.CombinedOutput("route", args...)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %s failed: %v (%s)", cidr, err, out)
	}
	return nil
}
