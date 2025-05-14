package route

import (
	"fmt"
	"golang.org/x/sync/errgroup"
	"os/exec"
	"strings"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func (w *Wrapper) Get(destIP string) error {
	parseRoute := func(target string) (gw, iface string, err error) {
		out, err := exec.Command("route", "-n", "get", target).CombinedOutput()
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

	gateway, iface, err := parseRoute(destIP)
	if err != nil {
		return err
	}

	if strings.HasPrefix(gateway, "127.") || gateway == "" {
		if gwDef, ifDef, err2 := parseRoute("default"); err2 == nil {
			gateway, iface = gwDef, ifDef
		}
	}

	switch {
	case gateway != "":
		if addErr := w.Add(gateway, iface); addErr != nil {
			return fmt.Errorf("route keep gw %s: %w", gateway, addErr)
		}
		return w.AddViaGateway(destIP, gateway)

	case iface != "":
		return w.Add(destIP, iface)

	default:
		return fmt.Errorf("no route found for %s", destIP)
	}
}

func (w *Wrapper) Add(ip, iface string) error {
	cmd := exec.Command("route", "add", ip, "-interface", iface)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add %s via interface %s failed: %v (%s)", ip, iface, err, out)
	}
	return nil
}

func (w *Wrapper) AddViaGateway(ip, gw string) error {
	cmd := exec.Command("route", "add", ip, gw)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add %s via %s failed: %v (%s)", ip, gw, err, out)
	}
	return nil
}

func (w *Wrapper) AddSplit(dev string) error {
	if out, err := exec.Command("route", "-q", "add", "-net", "0.0.0.0/1", "-interface", dev).CombinedOutput(); err != nil {
		return fmt.Errorf("route add 0.0.0.0/1 failed: %v (%s)", err, out)
	}
	if out, err := exec.Command("route", "-q", "add", "-net", "128.0.0.0/1", "-interface", dev).CombinedOutput(); err != nil {
		return fmt.Errorf("route add 128.0.0.0/1 failed: %v (%s)", err, out)
	}
	return nil
}

func (w *Wrapper) DelSplit(dev string) error {
	var errGroup errgroup.Group

	errGroup.Go(func() error {
		return exec.Command("route", "-q", "delete", "-net", "0.0.0.0/1", "-interface", dev).Run()
	})

	errGroup.Go(func() error {
		return exec.Command("route", "-q", "delete", "-net", "128.0.0.0/1", "-interface", dev).Run()
	})

	return errGroup.Wait()
}

func (w *Wrapper) Del(destIP string) error {
	cmd := exec.Command("route", "delete", destIP)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route delete %s failed: %v (%s)", destIP, err, out)
	}
	return nil
}

// DefaultGateway queries `route -n get default` to find the LAN gateway IP.
func (w *Wrapper) DefaultGateway() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("defaultGateway: %v (%s)", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "gateway:" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("defaultGateway: no gateway found")
}
