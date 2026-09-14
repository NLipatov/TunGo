package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParallelClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endpoint.json")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, path, "run/1", []string{"127.0.0.1"}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := range 6 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			if err := checkServer(path, "run/1"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRejectPreviousAttempt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endpoint.json")
	data, err := json.Marshal(endpoint{Marker: "run/1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkServer(path, "run/2"); err == nil || !strings.Contains(err.Error(), "different run attempt") {
		t.Fatalf("expected rejection before connecting, got %v", err)
	}
}

func TestRejectIncorrectReply(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = connection.Close() }()
		if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			done <- err
			return
		}
		if _, err := io.ReadFull(connection, make([]byte, len("right\n"))); err != nil {
			done <- err
			return
		}
		_, err = io.WriteString(connection, "wrong\n")
		done <- err
	}()
	if err := exchange("tcp4", listener.Addr().String(), "right\n"); err == nil || !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("expected incorrect reply rejection, got %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
