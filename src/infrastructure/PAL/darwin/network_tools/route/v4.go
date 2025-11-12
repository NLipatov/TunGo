//go:build darwin

package route

import (
	"fmt"
	"golang.org/x/sync/errgroup"
	"net"
	"strings"
	"tungo/infrastructure/PAL"
)

const (
	// v4SplitOne covers half of IPv4 address space
	// (addresses between 0.0.0.0 and 127.255.255.255)
	v4SplitOne = "0.0.0.0/1"
	// v4SplitTwo v4SplitOne covers half of IPv4 address space
	// (addresses between 128.0.0.0 and 255.255.255.255)
	v4SplitTwo = "128.0.0.0/1"
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
	if ip := net.ParseIP(destIP); ip == nil || ip.To4() == nil {
		return fmt.Errorf("v4.Get: non-IPv4 dest %q", destIP)
	}

	parseRoute := func(target string) (gw, iFace string, err error) {
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
		return
	}

	gw, iFace, err := parseRoute(destIP)
	if err != nil {
		return err
	}

	isLoop := strings.HasPrefix(gw, "127.")
	if (gw == "" && iFace == "") || isLoop {
		if gwDef, ifDef, err2 := parseRoute("default"); err2 == nil {
			if gwDef != "" && !strings.HasPrefix(gwDef, "127.") {
				gw, iFace = gwDef, ifDef
			}
		}
	}

	if gw != "" && !strings.HasPrefix(gw, "link#") {
		return v.AddViaGateway(destIP, gw)
	}
	if iFace != "" {
		return v.Add(destIP, iFace)
	}
	return fmt.Errorf("no route found for %s", destIP)
}

func (v *v4) Add(ip, iFace string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "add", ip, "-interface", iFace); err != nil {
		return fmt.Errorf("route add %s via interface %s failed: %v", ip, iFace, err)
	}
	return nil
}

func (v *v4) AddViaGateway(ip, gw string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "add", ip, gw); err != nil {
		return fmt.Errorf("route add %s via %s failed: %v", ip, gw, err)
	}
	return nil
}

func (v *v4) Del(destIP string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "delete", destIP); err != nil {
		return fmt.Errorf("route delete %s failed: %v", destIP, err)
	}
	return nil
}

func (v *v4) AddSplit(dev string) error {
	if _, err := v.commander.CombinedOutput("route", "-q", "add", "-net", v4SplitOne, "-interface", dev); err != nil {
		return fmt.Errorf("route add 0.0.0.0/1 failed: %v", err)
	}
	if _, err := v.commander.CombinedOutput("route", "-q", "add", "-net", v4SplitTwo, "-interface", dev); err != nil {
		return fmt.Errorf("route add 128.0.0.0/1 failed: %v", err)
	}
	return nil
}

func (v *v4) DelSplit(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error { return v.commander.Run("route", "-q", "delete", "-net", v4SplitOne, "-interface", dev) })
	eg.Go(func() error {
		return v.commander.Run("route", "-q", "delete", "-net", v4SplitTwo, "-interface", dev)
	})
	return eg.Wait()
}

func (v *v4) DefaultGateway() (string, error) {
	out, err := v.commander.CombinedOutput("route", "-n", "get", "default")
	if err != nil {
		return "", fmt.Errorf("defaultGateway: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "gateway:" {
			return f[1], nil
		}
	}
	return "", fmt.Errorf("defaultGateway: no gateway found")
}
