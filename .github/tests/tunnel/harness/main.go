package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	info, err := newRunInfo(os.Args[1:])
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
		defer stop()
		err = run(ctx, info)
	}
	if err != nil {
		slog.Error("Harness failed", "error", err)
		os.Exit(1)
	}
}

type runInfo struct {
	command, protocol, workdir string
}

func newRunInfo(args []string) (runInfo, error) {
	usage := fmt.Errorf("usage: tungo-e2e coordinator <UDP|TCP|WS> <workdir> | tungo-e2e client <workdir>")
	if len(args) < 2 {
		return runInfo{}, usage
	}
	info := runInfo{command: args[0], workdir: args[len(args)-1]}
	switch info.command {
	case "coordinator":
		if len(args) != 3 {
			return runInfo{}, usage
		}
		info.protocol = args[1]
		if info.protocol != "UDP" && info.protocol != "TCP" && info.protocol != "WS" {
			return runInfo{}, fmt.Errorf("invalid protocol %q", info.protocol)
		}
	case "client":
		if len(args) != 2 {
			return runInfo{}, usage
		}
	default:
		return runInfo{}, usage
	}
	if info.workdir == "" {
		return runInfo{}, fmt.Errorf("workdir is required")
	}
	return info, nil
}

func run(ctx context.Context, info runInfo) (err error) {
	directory, err := filepath.Abs(info.workdir)
	if err != nil {
		return err
	}
	logs := filepath.Join(directory, "logs")
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	if err := os.MkdirAll(logs, 0755); err != nil {
		return err
	}
	if info.command == "client" {
		return runClientCommand(ctx, directory, logs)
	}
	defer func() {
		if err != nil {
			printLogs(filepath.Join(logs, "*.log"))
		} else {
			fmt.Println("PASS")
		}
	}()
	slog.Info("Starting tunnel scenario", "protocol", info.protocol)
	server, err := connectServer(ctx, directory, logs)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, server.close()) }()
	return runTunnel(ctx, server, info.protocol, directory, logs)
}

func printLogs(pattern string) {
	paths, _ := filepath.Glob(pattern)
	for _, path := range paths {
		data, _ := os.ReadFile(path)
		if len(data) > 20000 {
			data = data[len(data)-20000:]
		}
		fmt.Fprintf(os.Stderr, "%s:\n%s\n", filepath.Base(path), data)
	}
}
