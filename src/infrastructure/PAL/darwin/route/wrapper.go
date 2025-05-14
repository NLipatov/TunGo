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
	// inner parseRoute now uses w.commander.CombinedOutput instead of exec.Command
	parseRoute := func(target string) (gw, iface string, err error) {
		out, err := w.commander.CombinedOutput("route", "-n", "get", target)
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
	_, err := w.commander.CombinedOutput("route", "add", ip, "-interface", iface)
	if err != nil {
		return fmt.Errorf("route add %s via interface %s failed: %v", ip, iface, err)
	}
	return nil
}

func (w *Wrapper) AddViaGateway(ip, gw string) error {
	_, err := w.commander.CombinedOutput("route", "add", ip, gw)
	if err != nil {
		return fmt.Errorf("route add %s via %s failed: %v", ip, gw, err)
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

func (w *Wrapper) Del(destIP string) error {
	_, err := w.commander.CombinedOutput("route", "delete", destIP)
	if err != nil {
		return fmt.Errorf("route delete %s failed: %v", destIP, err)
	}
	return nil
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
