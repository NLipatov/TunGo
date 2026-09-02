//go:build darwin

package route

import (
	"bytes"
	"fmt"
	"net/netip"
	"strings"

	"tungo/internal/tun/internal/splitroute"

	"golang.org/x/sync/errgroup"
)

const (
	loopbackIFaceNameV4 = "lo0"
	loopbackPrefixV4    = "127."
)

type v4Runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

type V4 struct {
	runner v4Runner
}

func NewV4(runner v4Runner) *V4 {
	return &V4{
		runner: runner,
	}
}

// Add installs a host route to destIP using its current gateway or interface.
func (v *V4) Add(destIP string) error {
	if ip, ipErr := netip.ParseAddr(destIP); ipErr != nil {
		return fmt.Errorf("v4.Add: invalid IP %q: %w", destIP, ipErr)
	} else if !ip.Is4() {
		return fmt.Errorf("v4.Add: non-IPv4 dest %q", destIP)
	} else if ip.IsLoopback() {
		return fmt.Errorf("v4.Add: invalid IP: loopback %q", destIP)
	}
	gateway, iFace, err := v.parseRoute(destIP)
	if err != nil {
		return err
	}
	// If route is empty or goes via loopback, try default route.
	if (gateway == "" && iFace == "") || v.isLoop(gateway, iFace) {
		if gwDef, ifDef, defErr := v.parseRoute("default"); defErr == nil {
			if gwDef != "" && !strings.HasPrefix(gwDef, loopbackPrefixV4) {
				gateway, iFace = gwDef, ifDef
			}
		}
	}
	// If still loopback after fallback – treat as an error.
	if v.isLoop(gateway, iFace) {
		return fmt.Errorf("v4.Add: no non-loopback route found for destination: %q", destIP)
	}
	// Delete old route to destIP, ignore possible errors.
	_ = v.Del(destIP)
	// Use the gateway when present; link# identifies an on-link route
	// that must be installed via the interface.
	if gateway != "" && !strings.HasPrefix(gateway, "link#") {
		return v.addViaGateway(destIP, gateway)
	}
	if iFace != "" {
		return v.addOnLink(destIP, iFace)
	}
	return fmt.Errorf("no route found for %s", destIP)
}

func (v *V4) isLoop(gateway, iFace string) bool {
	return iFace == loopbackIFaceNameV4 || strings.HasPrefix(gateway, loopbackPrefixV4)
}

func (v *V4) parseRoute(target string) (gw, iFace string, err error) {
	out, err := v.runner.CombinedOutput("route", "-n", "get", target)
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

func (v *V4) Del(destIP string) error {
	out, err := v.runner.CombinedOutput("route", "-q", "-n", "delete", destIP)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %s failed: %v (%s)", destIP, err, out)
	}
	return nil
}

func (v *V4) AddSplit(dev string) error {
	_ = v.runDeleteSplit("-net", splitroute.IPv4LowerHalf, "-interface", dev)
	_ = v.runDeleteSplit("-net", splitroute.IPv4UpperHalf, "-interface", dev)

	if out, err := v.runner.CombinedOutput("route", "-q", "-n", "add", "-net", splitroute.IPv4LowerHalf, "-interface", dev); err != nil &&
		!bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", splitroute.IPv4LowerHalf, err, out)
	}
	if out, err := v.runner.CombinedOutput("route", "-q", "-n", "add", "-net", splitroute.IPv4UpperHalf, "-interface", dev); err != nil &&
		!bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s failed: %v (%s)", splitroute.IPv4UpperHalf, err, out)
	}
	return nil
}

func (v *V4) DelSplit(dev string) error {
	var eg errgroup.Group
	eg.Go(func() error { return v.runDeleteSplit("-net", splitroute.IPv4LowerHalf, "-interface", dev) })
	eg.Go(func() error { return v.runDeleteSplit("-net", splitroute.IPv4UpperHalf, "-interface", dev) })
	return eg.Wait()
}

func (v *V4) addOnLink(ip, iFace string) error {
	out, err := v.runner.CombinedOutput("route", "-q", "-n", "add", ip, "-interface", iFace)
	if err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s via interface %s failed: %v (%s)", ip, iFace, err, out)
	}
	return nil
}

func (v *V4) addViaGateway(ip, gw string) error {
	out, err := v.runner.CombinedOutput("route", "-q", "-n", "add", ip, gw)
	if err != nil && !bytes.Contains(out, []byte("File exists")) {
		return fmt.Errorf("route add %s via %s failed: %v (%s)", ip, gw, err, out)
	}
	return nil
}

func (v *V4) runDeleteSplit(args ...string) error {
	full := append([]string{"-q", "-n", "delete"}, args...)
	out, err := v.runner.CombinedOutput("route", full...)
	if err != nil && !bytes.Contains(bytes.ToLower(out), []byte("not in table")) {
		return fmt.Errorf("route delete %v failed: %v (%s)", args, err, out)
	}
	return nil
}
