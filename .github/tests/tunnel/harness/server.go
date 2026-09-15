package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// server owns the SSH connection and the remote TunGo server process.
type server struct {
	sshClient      *ssh.Client
	sshSession     *ssh.Session
	networkBefore  string
	endpoint, logs string
}

//go:embed server_configuration.json
var serverConfiguration string

func connectServer(ctx context.Context, directory, logs string) (*server, error) {
	data, err := os.ReadFile(filepath.Join(directory, "server.json"))
	if err != nil {
		return nil, err
	}
	var addresses struct{ SSH, VPN string }
	if err := json.Unmarshal(data, &addresses); err != nil {
		return nil, err
	}
	if _, err := netip.ParseAddr(addresses.VPN); err != nil {
		return nil, fmt.Errorf("server VPN address: %w", err)
	}
	slog.Info("Server addresses", "vpn", addresses.VPN, "ssh", addresses.SSH)
	connection, err := connectSSH(ctx, directory, addresses.SSH, "root", "server.pub")
	if err != nil {
		return nil, err
	}
	return &server{sshClient: connection, endpoint: addresses.VPN, logs: logs}, nil
}

func (s *server) checkTarget(ctx context.Context) (string, error) {
	for _, f := range families {
		slog.Info("Checking server access to HTTP target", "family", f.number, "url", f.url("/peer"))
		if _, err := s.remote(ctx, "curl --noproxy '*' -fsS --max-time 5 '"+f.url("/peer")+"'"); err != nil {
			return "", err
		}
	}
	checksum, err := s.remote(ctx, "sha256sum /var/www/localhost/htdocs/payload")
	if err != nil {
		return "", err
	}
	checksum, _, _ = strings.Cut(checksum, " ")
	return checksum, nil
}

func (s *server) startServer(ctx context.Context, protocol string) (string, error) {
	before, err := s.remote(ctx, "network-state")
	if err != nil {
		return "", err
	}
	s.networkBefore = before
	env := fmt.Sprintf("export Host=%s EnableUDP=%t EnableTCP=%t EnableWS=%t\n",
		s.endpoint, protocol == "UDP", protocol == "TCP", protocol == "WS")
	// Create the fixture configuration without overwriting an existing server.
	setup := "umask 077\nmkdir -p /etc/tungo\nset -C\n" +
		"cat >/etc/tungo/server_configuration.json <<'TUNGO_CONFIG'\n" +
		serverConfiguration + "\nTUNGO_CONFIG\n"
	config, err := s.remote(ctx, setup+env+"tungo s gen >/dev/null\ncat /etc/tungo/client_configuration.json.1")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = s.sshClient.Close() })
	defer stop()
	s.sshSession, err = s.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	if err := s.sshSession.Start(env + "exec tungo s >/tmp/server.log 2>&1"); err != nil {
		return "", err
	}
	return config, nil
}

func (s *server) stopServer(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = s.sshClient.Close() })
	defer stop()
	if err := s.sshSession.Signal(ssh.SIGTERM); err != nil {
		return err
	}
	if err := s.sshSession.Wait(); err != nil {
		return err
	}
	after, err := s.remote(ctx, "network-state")
	if err != nil {
		return err
	}
	if after != s.networkBefore {
		return fmt.Errorf("server network not restored:\nbefore:\n%s\nafter:\n%s", s.networkBefore, after)
	}
	return nil
}

func (s *server) remote(ctx context.Context, script string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	// Closing the transport bounds channel creation as well as command execution.
	stop := context.AfterFunc(ctx, func() { _ = s.sshClient.Close() })
	defer stop()
	session, err := s.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()
	var stdout, stderr bytes.Buffer
	session.Stdout, session.Stderr = &stdout, &stderr
	if err := session.Run("set -eu\n" + script); err != nil {
		// stdout can contain enrollment secrets; never include it in errors.
		return "", fmt.Errorf("SSH command: %w: %s", errors.Join(err, ctx.Err()), stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (s *server) close() error {
	ctx := context.Background()
	var errs []error
	if s.sshSession != nil {
		_ = s.sshSession.Close()
	}
	if s.sshClient != nil {
		data, err := s.remote(ctx, `
network-state
ip -details -statistics link
iptables-save -c
ip6tables-save -c
for file in /tmp/server.log /tmp/sshd.log /tmp/target.log; do
    if [ -f "$file" ]; then
        printf '\n%s:\n' "$file"
        tail -c 12000 "$file"
    fi
done
`)
		if err == nil {
			err = os.WriteFile(filepath.Join(s.logs, "server.log"), []byte(data), 0644)
		}
		errs = append(errs, err)
		_ = s.sshClient.Close()
	}
	return errors.Join(errs...)
}
