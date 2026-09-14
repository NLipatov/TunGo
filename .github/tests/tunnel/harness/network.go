package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
)

type family struct {
	number int
	target string
	server string
	nat    string
}

var families = []family{
	{4, "198.18.0.2", "198.19.0.1", "198.18.0.1"},
	{6, "fd73:7467:6f::2", "fd73:7467:6f:1::1", "fd73:7467:6f::1"},
}

func (f family) url(path string) string {
	return "http://" + net.JoinHostPort(f.target, "8080") + path
}

func snapshot(ctx context.Context) (map[string][]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	state := map[string][]string{"interfaces": {}}
	for _, iface := range interfaces {
		state["interfaces"] = append(state["interfaces"], iface.Name)
	}
	sort.Strings(state["interfaces"])
	switch runtime.GOOS {
	case "linux":
		fields := []string{"dst", "gateway", "dev", "table", "metric", "prefsrc", "type", "scope"}
		for _, f := range families {
			rows, err := routeRows(ctx, "ip", "-j", fmt.Sprintf("-%d", f.number), "route", "show", "table", "main")
			if err != nil {
				return nil, err
			}
			state[fmt.Sprintf("routes%d", f.number)] = canonicalRows(rows, fields)
			firewall := "iptables"
			if f.number == 6 {
				firewall = "ip6tables"
			}
			for _, table := range []string{"filter", "nat", "mangle"} {
				output, err := command(ctx, firewall, "-t", table, "-S")
				if err != nil {
					return nil, err
				}
				key := fmt.Sprintf("firewall%d", f.number)
				state[key] = append(state[key], output)
			}
		}
	case "darwin":
		for _, name := range []string{"inet", "inet6"} {
			output, err := command(ctx, "netstat", "-rn", "-f", name)
			if err != nil {
				return nil, err
			}
			state[name] = darwinRoutes(output)
		}
	case "windows":
		output, err := powershell(ctx, "ConvertTo-Json -Compress -InputObject @(Get-NetRoute | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric)")
		if err != nil {
			return nil, err
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(output), &rows); err != nil {
			return nil, err
		}
		state["routes"] = canonicalRows(rows, nil)
	default:
		return nil, fmt.Errorf("unsupported runner OS %s", runtime.GOOS)
	}
	return state, nil
}

func assertTunnelRoute(ctx context.Context, f family) error {
	prefixes := []string{"0.0.0.0/1", "128.0.0.0/1"}
	if f.number == 6 {
		prefixes = []string{"::/1", "8000::/1"}
	}
	switch runtime.GOOS {
	case "linux":
		flag := fmt.Sprintf("-%d", f.number)
		target, err := routeRows(ctx, "ip", "-j", flag, "route", "get", f.target)
		if err != nil {
			return err
		}
		if len(target) == 0 || !strings.HasPrefix(fmt.Sprint(target[0]["dev"]), "c_") {
			return fmt.Errorf("target route does not use TunGo: %v", target)
		}
		routes, err := routeRows(ctx, "ip", "-j", flag, "route", "show")
		if err != nil {
			return err
		}
		for _, prefix := range prefixes {
			found := false
			for _, row := range routes {
				if row["dst"] == prefix && row["dev"] == target[0]["dev"] {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("missing tunnel route %s: %v", prefix, routes)
			}
		}
	case "darwin":
		args := []string{"route", "-n", "get"}
		name := "inet"
		if f.number == 6 {
			args = append(args, "-inet6")
			name = "inet6"
		} else {
			prefixes = []string{"0/1", "128.0/1"}
		}
		output, err := command(ctx, append(args, f.target)...)
		if err != nil {
			return err
		}
		iface, err := routeField(output, "interface")
		if err != nil {
			return err
		}
		if !strings.HasPrefix(iface, "utun") {
			return fmt.Errorf("target route uses %s instead of utun", iface)
		}
		output, err = command(ctx, "netstat", "-rn", "-f", name)
		if err != nil {
			return err
		}
		for _, prefix := range prefixes {
			found := false
			for _, line := range strings.Split(output, "\n") {
				parts := strings.Fields(line)
				if len(parts) >= 4 && parts[0] == prefix && parts[3] == iface {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("missing tunnel route %s on %s", prefix, iface)
			}
		}
	case "windows":
		iface, err := powershell(ctx, fmt.Sprintf("(Find-NetRoute -RemoteIPAddress '%s' | Where-Object { $_.PSObject.Properties['InterfaceAlias'] } | Select-Object -First 1).InterfaceAlias", f.target))
		if err != nil {
			return err
		}
		if !strings.HasPrefix(iface, "c_") {
			return fmt.Errorf("target route does not use TunGo: %s", iface)
		}
		for _, prefix := range prefixes {
			aliases, err := powershell(ctx, fmt.Sprintf("(Get-NetRoute -DestinationPrefix '%s').InterfaceAlias", prefix))
			if err != nil {
				return err
			}
			found := false
			for _, alias := range strings.Split(aliases, "\n") {
				if strings.TrimSpace(alias) == iface {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("missing tunnel route %s on %s", prefix, iface)
			}
		}
	}
	return nil
}

func routeRows(ctx context.Context, args ...string) ([]map[string]any, error) {
	output, err := command(ctx, args...)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, fmt.Errorf("parse route output: %w: %s", err, output)
	}
	return rows, nil
}

func canonicalRows(rows []map[string]any, fields []string) []string {
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		if fields != nil {
			selected := make(map[string]any)
			for _, field := range fields {
				if value, ok := row[field]; ok {
					selected[field] = value
				}
			}
			row = selected
		}
		encoded, _ := json.Marshal(row)
		result = append(result, string(encoded))
	}
	sort.Strings(result)
	return result
}

func darwinRoutes(output string) []string {
	var rows []string
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 4 || !strings.ContainsRune("0123456789abcdef", rune(parts[0][0])) {
			continue
		}
		// Omit neighbor entries and cloned host routes, whose lifetimes vary.
		if !strings.ContainsAny(parts[2], "LW") {
			rows = append(rows, strings.Join(parts[:4], " "))
		}
	}
	sort.Strings(rows)
	return rows
}

func routeField(output, field string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[0] == field+":" {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("missing %s in route: %s", field, output)
}
