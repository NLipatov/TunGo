package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

func connectSSH(ctx context.Context, directory, address, user, hostKeyFile string) (*ssh.Client, error) {
	privateKey, err := os.ReadFile(filepath.Join(directory, "tungo-ssh", "identity"))
	if err != nil {
		return nil, err
	}
	defer clear(privateKey)
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	publicKey, err := os.ReadFile(filepath.Join(directory, "tungo-ssh", hostKeyFile))
	if err != nil {
		return nil, err
	}
	hostKey, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}
	var connection *ssh.Client
	if err := waitUntil(ctx, "SSH "+address, 30*time.Second, func(ctx context.Context) error {
		connection, err = dialSSH(ctx, address, config)
		return err
	}); err != nil {
		return nil, err
	}
	return connection, nil
}

func dialSSH(ctx context.Context, address string, config *ssh.ClientConfig) (*ssh.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SSH handshake: %w", err)
	}
	if !stop() {
		_ = client.Close()
		return nil, ctx.Err()
	}
	return ssh.NewClient(client, channels, requests), nil
}
