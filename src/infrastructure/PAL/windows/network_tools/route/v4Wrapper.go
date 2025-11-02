//go:build windows

package route

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

type v4Wrapper struct {
	commander PAL.Commander
}

func newV4Wrapper(c PAL.Commander) Contract { return &v4Wrapper{commander: c} }

// DefaultRoute parses `route print -4` and picks the row with the lowest Metric.
// Columns (locale-agnostic tokens): Destination, Netmask, Gateway, Interface-IP, Metric.
// We match only lines starting with "0.0.0.0 0.0.0.0".
func (w *v4Wrapper) DefaultRoute() (gw, ifName string, metric int, err error) {
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

func (w *v4Wrapper) Delete(dst string) error {
	dst = strings.TrimSpace(dst)
	if dst == "" {
		return fmt.Errorf("route delete: empty destination")
	}
	out, err := w.commander.CombinedOutput("route", "delete", dst)
	if err != nil {
		return fmt.Errorf("route delete %s: %v, output: %s", dst, err, out)
	}
	return nil
}

func (w *v4Wrapper) Print(t string) ([]byte, error) {
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

type psBestRoute struct {
	NextHop        string `json:"NextHop"`
	InterfaceAlias string `json:"InterfaceAlias"`
	InterfaceIndex int    `json:"InterfaceIndex"`
	RouteMetric    int    `json:"RouteMetric"`
}

// BestRoute resolves the effective IPv4 route using Find-NetRoute.
// Compatible with older NetTCPIP (no -AddressFamily).
func (w *v4Wrapper) BestRoute(dest string) (string, string, int, int, error) {
	ip := strings.TrimSpace(dest)
	p := net.ParseIP(ip)
	if p == nil || p.To4() == nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v4): not an IPv4 address: %q", dest)
	}

	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$ip = '%s'
$cmd = Get-Command -Name Find-NetRoute -ErrorAction SilentlyContinue
if (-not $cmd) { exit 3 }
try {
    if ($cmd.Parameters.ContainsKey('AddressFamily')) {
        $r = Find-NetRoute -RemoteIPAddress $ip -AddressFamily IPv4
    } else {
        $r = Find-NetRoute -RemoteIPAddress $ip
    }
} catch [System.Management.Automation.ParameterBindingException] {
    # старый синтаксис — без AddressFamily
    $r = Find-NetRoute -RemoteIPAddress $ip
}
if (-not $r) { exit 2 }
$r = $r | ForEach-Object {
    $_ | Add-Member -NotePropertyName PL -NotePropertyValue ([int](($_.DestinationPrefix -split '/')[1])) -PassThru
} | Sort-Object -Property @{Expression={-($_.PL)}}, RouteMetric | Select-Object -First 1
$r | Select-Object NextHop,InterfaceAlias,InterfaceIndex,RouteMetric | ConvertTo-Json -Compress
`, psQuote(ip))

	out, err := w.commander.CombinedOutput("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v4): Find-NetRoute error: %v, output: %s", err, out)
	}
	var br psBestRoute
	if uerr := json.Unmarshal(out, &br); uerr != nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v4): parse error: %v, output: %s", uerr, out)
	}
	gw := strings.TrimSpace(br.NextHop)
	if gw == "" || gw == "0.0.0.0" {
		gw = ""
	}
	return gw, br.InterfaceAlias, br.InterfaceIndex, br.RouteMetric, nil
}

func psQuote(s string) string { // single-quote for PowerShell literal
	return strings.ReplaceAll(s, `'`, `''`)
}
