package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChildGracefulShutdown(t *testing.T) {
	directory := t.TempDir()
	logPath, ready := filepath.Join(directory, "child.log"), filepath.Join(directory, "ready")
	t.Setenv("TUNGO_E2E_CHILD_TEST", "graceful")
	t.Setenv("TUNGO_E2E_CHILD_READY", ready)
	p, err := startChild(logPath, os.Args[0], "-test.run=^TestChildHelper$")
	if err != nil {
		t.Fatal(err)
	}
	defer p.kill()
	if err := waitUntil(context.Background(), "child signal handler ready", 10*time.Second, func(context.Context) error {
		_, err := os.Stat(ready)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.stop(); err != nil {
		t.Fatal(err)
	}
	if p.running() {
		t.Fatal("child still running after stop")
	}
	body, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(body), "graceful cleanup complete") {
		t.Fatalf("shutdown skipped cleanup: %q, %v", body, err)
	}
	if err := p.stop(); err == nil {
		t.Fatal("premature exit accepted as graceful shutdown")
	}
}

func TestChildFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "child.log")
	t.Setenv("TUNGO_E2E_CHILD_TEST", "failure")
	p, err := startChild(logPath, os.Args[0], "-test.run=^TestChildHelper$")
	if err != nil {
		t.Fatal(err)
	}
	defer p.kill()
	if err := p.wait(10 * time.Second); err == nil {
		t.Fatal("child failure accepted")
	}
	if p.running() {
		t.Fatal("wait returned before child exited")
	}
	if err := p.stop(); err == nil {
		t.Fatal("failed child accepted as graceful shutdown")
	}
	body, err := os.ReadFile(logPath)
	if err != nil || string(body) != "child failed\n" {
		t.Fatalf("child diagnostics lost: %q, %v", body, err)
	}
}

func TestChildHelper(t *testing.T) {
	mode := os.Getenv("TUNGO_E2E_CHILD_TEST")
	if mode == "" {
		return
	}
	if mode == "failure" {
		fmt.Println("child failed")
		os.Exit(7)
	}
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	if err := os.WriteFile(os.Getenv("TUNGO_E2E_CHILD_READY"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	fmt.Println("graceful cleanup complete")
}
