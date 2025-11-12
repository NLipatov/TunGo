//go:build darwin

package route

import (
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
	}

	parseRoute := func(target string) (gw, iface string, err error) {
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
		return
	}

	gw, iface, err := parseRoute(destIP)
	if err != nil {
		return err
	}
	isLoop := gw == "::1"
	if (gw == "" && iface == "") || isLoop {
		if gwDef, ifDef, err2 := parseRoute("default"); err2 == nil {
			if gwDef != "" && gwDef != "::1" {
				gw, iface = gwDef, ifDef
			}
		}
	}

	if strings.HasPrefix(gw, "fe80:") && !strings.Contains(gw, "%") && iface != "" {
		gw = gw + "%" + iface
	}

	switch {
	case gw != "" && !strings.HasPrefix(gw, "link#"):
		return v.AddViaGateway(destIP, gw)
	case iface != "":
		return v.Add(destIP, iface)
	default:
		return fmt.Errorf("no route found for %s", destIP)
	}
}

func (v *v6) Add(ip, iface string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", ip, "-interface", iface); err != nil {
		return fmt.Errorf("route add %s via interface %s failed: %v", ip, iface, err)
	}
	return nil
}

func (v *v6) AddViaGateway(ip, gw string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", ip, gw); err != nil {
		return fmt.Errorf("route add %s via %s failed: %v", ip, gw, err)
	}
	return nil
}

func (v *v6) Del(destIP string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "-n", "delete", "-inet6", destIP); err != nil {
		return fmt.Errorf("route delete %s failed: %v", destIP, err)
	}
	return nil
}

func (v *v6) AddSplit(dev string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", v6SplitOne, "-interface", dev); err != nil {
		return fmt.Errorf("route add ::/1 failed: %v", err)
	}
	if _, err := v.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", v6SplitTwo, "-interface", dev); err != nil {
		return fmt.Errorf("route add 8000::/1 failed: %v", err)
	}
	return nil
}

func (v *v6) DelSplit(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error {
		return v.commander.Run("route", "-q", "-n", "delete", "-inet6", v6SplitOne, "-interface", dev)
	})
	eg.Go(func() error {
		return v.commander.Run("route", "-q", "-n", "delete", "-inet6", v6SplitTwo, "-interface", dev)
	})
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
