package route

import (
	"fmt"
	"golang.org/x/sync/errgroup"
	"strings"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) Contract {
	return &Wrapper{commander: commander}
}

func (w *Wrapper) Get(destIP string) error {
	parseRoute := func(target string, v6 bool) (gw, iface string, err error) {
		args := []string{"-n", "get", target}
		if v6 {
			args = []string{"-n", "-inet6", "get", target}
		}
		out, err := w.commander.CombinedOutput("route", args...)
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

	v6 := strings.Contains(destIP, ":")
	gateway, iface, err := parseRoute(destIP, v6)
	if err != nil {
		return err
	}

	isLoop := strings.HasPrefix(gateway, "127.") || gateway == "::1"
	hasRealGW := gateway != "" && !isLoop && !strings.HasPrefix(gateway, "link#")
	if !hasRealGW {
		if gwDef, ifDef, err2 := parseRoute("default", v6); err2 == nil {
			isLoopDef := strings.HasPrefix(gwDef, "127.") || gwDef == "::1"
			if gwDef != "" && !isLoopDef && !strings.HasPrefix(gwDef, "link#") {
				gateway, iface = gwDef, ifDef
				hasRealGW = true
			}
		}
	}

	if v6 && strings.HasPrefix(gateway, "fe80:") && !strings.Contains(gateway, "%") && iface != "" {
		gateway = gateway + "%" + iface
	}

	switch {
	case hasRealGW:
		if err := w.Add(gateway, iface); err != nil {
			return fmt.Errorf("pin gateway %s via %s failed: %w", gateway, iface, err)
		}
		return w.AddViaGateway(destIP, gateway)
	case iface != "":
		return w.Add(destIP, iface)
	default:
		return fmt.Errorf("no route found for %s", destIP)
	}
}

func (w *Wrapper) Add(ip, iface string) error {
	var args []string
	if strings.Contains(ip, ":") {
		args = []string{"-q", "-n", "add", "-inet6", ip, "-interface", iface}
	} else {
		args = []string{"-q", "add", ip, "-interface", iface}
	}
	if _, err := w.commander.CombinedOutput("route", args...); err != nil {
		return fmt.Errorf("route add %s via interface %s failed: %v", ip, iface, err)
	}
	return nil
}

func (w *Wrapper) AddViaGateway(ip, gw string) error {
	var args []string
	if strings.Contains(ip, ":") || strings.Contains(gw, ":") {
		args = []string{"-q", "-n", "add", "-inet6", ip, gw}
	} else {
		args = []string{"-q", "add", ip, gw}
	}
	if _, err := w.commander.CombinedOutput("route", args...); err != nil {
		return fmt.Errorf("route add %s via %s failed: %v", ip, gw, err)
	}
	return nil
}

func (w *Wrapper) Del(destIP string) error {
	var args []string
	if strings.Contains(destIP, ":") {
		args = []string{"-q", "-n", "delete", "-inet6", destIP}
	} else {
		args = []string{"-q", "delete", destIP}
	}
	if _, err := w.commander.CombinedOutput("route", args...); err != nil {
		return fmt.Errorf("route delete %s failed: %v", destIP, err)
	}
	return nil
}

func (w *Wrapper) AddSplit(dev string) error {
	if _, err := w.commander.CombinedOutput("route", "-q", "add", "-net", "0.0.0.0/1", "-interface", dev); err != nil {
		return fmt.Errorf("route add 0.0.0.0/1 failed: %v", err)
	}
	if _, err := w.commander.CombinedOutput("route", "-q", "add", "-net", "128.0.0.0/1", "-interface", dev); err != nil {
		return fmt.Errorf("route add 128.0.0.0/1 failed: %v", err)
	}
	return nil
}

func (w *Wrapper) AddSplitV6(dev string) error {
	if _, err := w.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", "::/1", "-interface", dev); err != nil {
		return fmt.Errorf("route add ::/1 failed: %v", err)
	}
	if _, err := w.commander.CombinedOutput("route", "-q", "-n", "add", "-inet6", "8000::/1", "-interface", dev); err != nil {
		return fmt.Errorf("route add 8000::/1 failed: %v", err)
	}
	return nil
}

func (w *Wrapper) DelSplitV6(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error {
		return w.commander.Run("route", "-q", "-n", "delete", "-inet6", "::/1", "-interface", dev)
	})
	eg.Go(func() error {
		return w.commander.Run("route", "-q", "-n", "delete", "-inet6", "8000::/1", "-interface", dev)
	})
	return eg.Wait()
}

func (w *Wrapper) DelSplit(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error {
		return w.commander.Run("route", "-q", "delete", "-net", "0.0.0.0/1", "-interface", dev)
	})
	eg.Go(func() error {
		return w.commander.Run("route", "-q", "delete", "-net", "128.0.0.0/1", "-interface", dev)
	})
	return eg.Wait()
}

func (w *Wrapper) DefaultGateway() (string, error) {
	out, err := w.commander.CombinedOutput("route", "-n", "get", "default")
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

func (w *Wrapper) DefaultGatewayV6() (string, error) {
	out, err := w.commander.CombinedOutput("route", "-n", "-inet6", "get", "default")
	if err != nil {
		return "", fmt.Errorf("defaultGatewayV6: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "gateway:" {
			return f[1], nil
		}
	}
	return "", fmt.Errorf("defaultGatewayV6: no gateway found")
}
