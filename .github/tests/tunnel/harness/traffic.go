package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var families = []family{
	{4, "198.18.0.2", "198.19.0.1", "198.19.0.2", "198.18.0.1"},
	{6, "fd73:7467:6f::2", "fd73:7467:6f:1::1", "fd73:7467:6f:1::2", "fd73:7467:6f::1"},
}

func checkTraffic(ctx context.Context, f family, checksum string) error {
	flag := fmt.Sprintf("-%d", f.number)
	ping := []string{"ping", flag, "-n", "-c", "1", f.target}
	trace := []string{"traceroute", flag, "-I", "-n", "-m", "2", "-q", "1", "-w", "2", f.target}
	switch runtime.GOOS {
	case "darwin":
		ping = []string{"ping", "-n", "-c", "1", f.target}
		trace = []string{"traceroute", "-I", "-n", "-m", "2", "-q", "1", "-w", "2", f.target}
		if f.number == 6 {
			ping[0], trace[0] = "ping6", "traceroute6"
		}
	case "windows":
		ping = []string{"ping", flag, "-n", "1", "-w", "2000", f.target}
		trace = []string{"tracert", flag, "-d", "-h", "2", "-w", "2000", f.target}
	}
	slog.Info("Checking TUN routes and target ping", "family", f.number, "client_tun", f.client, "target", f.target)
	if err := waitUntil(ctx, "IPv"+fmt.Sprint(f.number)+" ping through tunnel", 60*time.Second, func(ctx context.Context) error {
		// A reachable address alone is not proof that the tunnel is ready.
		// Check both halves of the split default without sending internet traffic.
		for destination, source := range routeSources(ctx) {
			if netip.MustParseAddr(destination).Is4() == (f.number == 4) && source != f.client {
				return fmt.Errorf("route to %s selects %s, want TUN source %s", destination, source, f.client)
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, err := command(ctx, ping...)
		return err
	}); err != nil {
		return err
	}
	slog.Info("Checking traceroute through server to target", "family", f.number, "hop_1", f.server, "hop_2", f.target)
	output, err := command(ctx, trace...)
	if err != nil {
		return err
	}
	fmt.Printf("IPv%d traceroute:\n%s\n", f.number, output)
	if !traceHop(output, 1, f.server) || !traceHop(output, 2, f.target) {
		return fmt.Errorf("trace must go through %s to %s", f.server, f.target)
	}
	slog.Info("Checking 4 MiB HTTP payload and SHA-256", "family", f.number, "target", f.target)
	curl := exec.CommandContext(ctx, "curl", "--noproxy", "*", "-fsS",
		"--connect-timeout", "2", "--max-time", "20", f.url("/payload"))
	curl.Stderr = os.Stderr
	body, err := curl.Output()
	if err != nil {
		return fmt.Errorf("IPv%d payload download: %w", f.number, err)
	}
	if len(body) != 4*1024*1024 || fmt.Sprintf("%x", sha256.Sum256(body)) != checksum {
		return fmt.Errorf("IPv%d payload size or checksum mismatch", f.number)
	}
	slog.Info("Checking target sees server NAT source", "family", f.number, "target", f.target, "source", f.nat)
	peer, err := command(ctx, "curl", "--noproxy", "*", "-fsS", "--max-time", "5", f.url("/peer"))
	if err != nil {
		return err
	}
	if peer != f.nat {
		return fmt.Errorf("IPv%d target sees %s, want NAT source %s", f.number, peer, f.nat)
	}
	return nil
}

type family struct {
	number                      int
	target, server, client, nat string
}

func (f family) url(path string) string {
	return "http://" + net.JoinHostPort(f.target, "8080") + path
}

func routeSources(ctx context.Context) map[string]string {
	sources := make(map[string]string)
	for _, destination := range []string{"1.1.1.1", "203.0.113.1", "2001:db8::1", "fd00:1234::1"} {
		// UDP connect asks the kernel for a route; no packet is sent.
		conn, err := (&net.Dialer{}).DialContext(ctx, "udp", net.JoinHostPort(destination, "9"))
		if err != nil {
			sources[destination] = "unreachable"
			continue
		}
		sources[destination] = conn.LocalAddr().(*net.UDPAddr).IP.String()
		_ = conn.Close()
	}
	return sources
}

func traceHop(output string, hop int, address string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != fmt.Sprint(hop) {
			continue
		}
		for _, field := range fields[1:] {
			if ip, err := netip.ParseAddr(strings.Trim(field, "()[]")); err == nil && ip == netip.MustParseAddr(address) {
				return true
			}
		}
	}
	return false
}

func clientInterface() (net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			return net.Interface{}, err
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil {
				return net.Interface{}, err
			}
			for _, f := range families {
				if prefix.Addr() == netip.MustParseAddr(f.client) {
					return iface, nil
				}
			}
		}
	}
	return net.Interface{}, fmt.Errorf("client TUN interface not found")
}

func checkClientCleanup(ctx context.Context, tun net.Interface, baseline map[string]string) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range interfaces {
		if iface.Index == tun.Index && iface.Name == tun.Name {
			return fmt.Errorf("client TUN interface %s (index %d) remains", tun.Name, tun.Index)
		}
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return err
	}
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err != nil {
			return err
		}
		for _, f := range families {
			if prefix.Addr() == netip.MustParseAddr(f.client) {
				return fmt.Errorf("client TUN address %s remains", f.client)
			}
		}
	}
	if after := routeSources(ctx); !maps.Equal(after, baseline) {
		return fmt.Errorf("route sources not restored: before=%v, after=%v", baseline, after)
	}
	return nil
}

func assertUnreachable(ctx context.Context, f family) error {
	slog.Info("Checking target is unreachable without the tunnel", "family", f.number, "target", f.target)
	if _, err := command(ctx, "curl", "--noproxy", "*", "-fsS", "--max-time", "2", f.url("/peer")); err == nil {
		return fmt.Errorf("IPv%d target reachable without tunnel", f.number)
	}
	return ctx.Err()
}

func waitUntil(ctx context.Context, description string, timeout time.Duration, action func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w; last error: %v", description, err, last)
		}
		if last = action(ctx); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w; last error: %v", description, ctx.Err(), last)
		case <-time.After(500 * time.Millisecond):
		}
	}
}
