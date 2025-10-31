//go:build windows

package route

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

type V4Wrapper struct {
	commander PAL.Commander
}

func NewV4Wrapper(c PAL.Commander) Contract {
	return &V4Wrapper{
		commander: c,
	}
}

// DefaultRoute parses `route print -4` and picks the row with the lowest Metric:
// Columns (locale-agnostic tokens): Destination, Netmask, Gateway, Interface-IP, Metric.
func (w *V4Wrapper) DefaultRoute() (gw, ifName string, metric int, err error) {
	out, execErr := w.commander.CombinedOutput("route", "print", "-4")
	if execErr != nil {
		return "", "", 0, fmt.Errorf("route print -4: %w", execErr)
	}
	best := int(^uint(0) >> 1)
	var bestGW, bestIfIP string

	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// Expect at least 5 columns; match "0.0.0.0" default with "0.0.0.0" netmask.
		if len(f) < 5 || f[0] != "0.0.0.0" || f[1] != "0.0.0.0" {
			continue
		}
		gwIP := net.ParseIP(f[2]).To4()
		if gwIP == nil || gwIP.IsUnspecified() {
			continue
		}
		met, _ := strconv.Atoi(f[len(f)-1])
		if met < best {
			best = met
			bestGW = gwIP.String()
			bestIfIP = f[3]
		}
	}
	if bestGW == "" || bestIfIP == "" {
		return "", "", 0, errors.New("default v4 route not found")
	}
	ifName, nameErr := iFaceNameByIP(bestIfIP)
	if nameErr != nil {
		return "", "", 0, nameErr
	}
	return bestGW, ifName, best, nil
}

func (w *V4Wrapper) Delete(dst string) error {
	out, err := w.commander.CombinedOutput("route", "delete", dst)
	if err != nil {
		return fmt.Errorf("route delete %s: %v, output: %s", dst, err, out)
	}
	return nil
}

func (w *V4Wrapper) Print(t string) ([]byte, error) {
	args := []string{"print", "-4"}
	if s := strings.TrimSpace(t); s != "" {
		args = append(args, s)
	}
	out, err := w.commander.CombinedOutput("route", args...)
	if err != nil {
		return nil, fmt.Errorf("route %s: %v, output: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func iFaceNameByIP(ip string) (string, error) {
	want := net.ParseIP(strings.TrimSpace(ip))
	if want == nil {
		return "", fmt.Errorf("bad ip: %s", ip)
	}
	ifs, _ := net.Interfaces()
	for _, it := range ifs {
		addresses, _ := it.Addrs()
		for _, a := range addresses {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(want) {
				return it.Name, nil
			}
		}
	}
	return "", fmt.Errorf("iface not found for %s", ip)
}
