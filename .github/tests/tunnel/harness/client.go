package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// clientInput is sent through SSH stdin, never through arguments or log files.
type clientInput struct {
	Config, Checksum string
}

func runRemoteClient(ctx context.Context, directory, artifacts string, input clientInput) (err error) {
	data, err := os.ReadFile(filepath.Join(directory, "client.json"))
	if err != nil {
		return err
	}
	var host struct{ SSH, User, Command string }
	if err := json.Unmarshal(data, &host); err != nil {
		return err
	}
	connection, err := connectSSH(ctx, directory, host.SSH, host.User, "client.pub")
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	// Bound session creation, execution and output collection on connection loss.
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	session, err := connection.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	data, err = json.Marshal(input)
	if err != nil {
		return err
	}
	log, err := os.Create(filepath.Join(artifacts, "ssh-client.log"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, log.Close()) }()
	session.Stdin = bytes.NewReader(data)
	session.Stdout = io.MultiWriter(os.Stdout, log)
	session.Stderr = io.MultiWriter(os.Stderr, log)
	if err := session.Run(host.Command); err != nil {
		return fmt.Errorf("client scenario over SSH: %w", errors.Join(err, ctx.Err()))
	}
	return nil
}

func runClientCommand(ctx context.Context, directory, artifacts string) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var input clientInput
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1024*1024)).Decode(&input); err != nil {
		return fmt.Errorf("client input: %w", err)
	}
	if err := prepareRunner(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			printLogs(filepath.Join(artifacts, "client-*.log"))
		}
	}()
	binary := filepath.Join(directory, "tungo-client")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	fmt.Printf("Client %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return runClient(ctx, binary, artifacts, input.Config, input.Checksum)
}

func runClient(ctx context.Context, binary, artifacts, config, checksum string) (err error) {
	// A healthy target must remain isolated until the client connects.
	for _, f := range families {
		if err := assertUnreachable(ctx, f); err != nil {
			return err
		}
	}
	baseline := routeSources(ctx)
	path, err := installConfig(config)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.Remove(path)) }()

	// Reuse the same configuration and server for a second connection.
	for cycle := 1; cycle <= 2; cycle++ {
		fmt.Printf("Connection %d\n", cycle)
		log := filepath.Join(artifacts, fmt.Sprintf("client-%d.log", cycle))
		if err := checkConnection(ctx, binary, log, checksum, baseline); err != nil {
			return fmt.Errorf("connection %d: %w", cycle, err)
		}
	}
	return nil
}

func installConfig(config string) (string, error) {
	path := "/etc/tungo/client_configuration.json"
	if runtime.GOOS == "windows" {
		directory := os.Getenv("ProgramData")
		if directory == "" {
			return "", fmt.Errorf("ProgramData is not set")
		}
		path = filepath.Join(directory, "TunGo", "client_configuration.json")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.WriteString(config)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

func checkConnection(ctx context.Context, binary, log, checksum string, baseline map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	client, err := startChild(log, binary, "c")
	if err != nil {
		return err
	}
	defer client.kill()
	for _, f := range families {
		if err := checkTraffic(ctx, f, checksum); err != nil {
			return err
		}
	}
	if err := client.stop(); err != nil {
		return err
	}
	if err := waitUntil(ctx, "client TUN and routes removed", 15*time.Second, func(ctx context.Context) error {
		return checkClientCleanup(ctx, baseline)
	}); err != nil {
		return err
	}
	for _, f := range families {
		if err := assertUnreachable(ctx, f); err != nil {
			return err
		}
	}
	return nil
}
