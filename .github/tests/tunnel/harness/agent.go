package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

func runAgent(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("agent requires Linux")
	}
	a := &agent{}
	defer func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.server != nil && a.server.running() {
			_ = a.server.stop()
			a.server.kill()
		}
	}()
	return serve(ctx, "tcp4", "192.168.250.15:18080", a)
}

// The agent owns one server process and its pre-start network state.
// HTTP requests are serialized, including diagnostics during start and stop.
type agent struct {
	mu       sync.Mutex
	server   *child
	baseline map[string][]string
	protocol string
}

func (a *agent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, err := a.handle(r)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		data = map[string]string{"error": err.Error()}
	}
	_ = json.NewEncoder(w).Encode(data)
}

func (a *agent) handle(r *http.Request) (any, error) {
	ctx := r.Context()
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/status":
			var checksum string
			for _, f := range families {
				body, err := request(ctx, f.url("/sha256"), nil, 5*time.Second)
				if err != nil {
					return nil, err
				}
				value := strings.TrimSpace(string(body))
				sum, err := hex.DecodeString(value)
				if err != nil || len(sum) != sha256.Size {
					return nil, fmt.Errorf("IPv%d HTTP target returned an invalid SHA-256", f.number)
				}
				if checksum != "" && value != checksum {
					return nil, fmt.Errorf("IPv4 and IPv6 HTTP target checksums differ")
				}
				checksum = value
			}
			var exit any
			if a.server != nil && !a.server.running() {
				exit = a.server.cmd.ProcessState.ExitCode()
			}
			arch, err := command(ctx, "uname", "-m")
			if err != nil {
				return nil, err
			}
			return map[string]any{"arch": arch, "sha256": checksum, "server_exit": exit}, nil
		case "/diagnostics":
			state, err := snapshot(ctx)
			if err != nil {
				return nil, err
			}
			data := map[string]any{"network": state}
			for key, args := range map[string][]string{
				"links":    {"ip", "-details", "-statistics", "link"},
				"counters": {"iptables-save", "-c"}, "counters6": {"ip6tables-save", "-c"},
			} {
				data[key], err = command(ctx, args...)
				if err != nil {
					return nil, err
				}
			}
			for key, path := range map[string]string{
				"server_log": "/tmp/server.log", "target_log": "/tmp/target.log",
				"target6_log": "/tmp/target6.log", "udp_packets": "/tmp/udp.log",
			} {
				body, err := os.ReadFile(path)
				if err != nil && !os.IsNotExist(err) {
					return nil, err
				}
				if key == "udp_packets" && len(body) > 12000 {
					body = body[len(body)-12000:]
				}
				data[key] = string(body)
			}
			return data, nil
		}
	}
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("unknown endpoint")
	}
	if r.ContentLength <= 0 || r.ContentLength >= 4096 {
		return nil, fmt.Errorf("invalid request size")
	}
	var data struct{ Protocol, Host string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&data); err != nil {
		return nil, err
	}
	switch r.URL.Path {
	case "/start":
		if a.server != nil {
			return nil, fmt.Errorf("server already started")
		}
		if data.Protocol != "UDP" && data.Protocol != "TCP" && data.Protocol != "WS" {
			return nil, fmt.Errorf("invalid protocol")
		}
		if data.Host != "127.77.0.1" && data.Host != "192.168.250.15" {
			return nil, fmt.Errorf("invalid transport host")
		}
		baseline, err := snapshot(ctx)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll("/etc/tungo", 0700); err != nil {
			return nil, err
		}
		settings := make(map[string]any)
		env := append(os.Environ(), "Host="+data.Host)
		for _, protocol := range []string{"UDP", "TCP", "WS"} {
			settings[protocol+"Settings"] = map[string]string{
				"IPv4Subnet": "198.19.0.0/24", "IPv6Subnet": "fd73:7467:6f:1::/64",
			}
			env = append(env, fmt.Sprintf("Enable%s=%t", protocol, protocol == data.Protocol))
		}
		body, err := json.Marshal(settings)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile("/etc/tungo/server_configuration.json", body, 0600); err != nil {
			return nil, err
		}
		// Generate enrollment only after boot; never include it in VM artifacts or logs.
		generateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		generate := exec.CommandContext(generateCtx, "tungo", "s", "gen")
		generate.Env, generate.Stdout, generate.Stderr = env, io.Discard, os.Stderr
		if err := generate.Run(); err != nil {
			return nil, fmt.Errorf("generate client: %w", err)
		}
		config, err := os.ReadFile("/etc/tungo/client_configuration.json.1")
		if err != nil {
			return nil, err
		}
		if !json.Valid(config) {
			return nil, fmt.Errorf("generated client configuration is invalid JSON")
		}
		server, err := startChild("/tmp/server.log", env, "tungo", "s")
		if err != nil {
			return nil, err
		}
		a.server, a.baseline, a.protocol = server, baseline, data.Protocol
		return json.RawMessage(config), nil
	case "/stop":
		if a.server == nil {
			return nil, fmt.Errorf("server not started")
		}
		if err := a.server.stop(); err != nil {
			return nil, err
		}
		after, err := snapshot(ctx)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(after, a.baseline) {
			return nil, fmt.Errorf("server state not restored: before=%v, after=%v", a.baseline, after)
		}
		name := "s_" + strings.ToLower(a.protocol) + "tun0"
		if _, err := command(ctx, "ip", "link", "show", name); err == nil {
			return nil, fmt.Errorf("leftover TUN: %s", name)
		}
		return map[string]bool{"restored": true}, nil
	}
	return nil, fmt.Errorf("unknown endpoint")
}
