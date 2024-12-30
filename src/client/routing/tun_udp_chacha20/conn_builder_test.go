package tun_udp_chacha20

import (
	"context"
	"net"
	"testing"
	"time"
	"tungo/settings"
)

func TestConnectionBuilder_NewInstance(t *testing.T) {
	builder := newConnectionBuilder()

	if builder == nil {
		t.Fatal("Expected newConnectionBuilder to return an instance, got nil")
	}
}

func TestConnectionBuilder_UseSettings(t *testing.T) {
	connectionSettings := settings.ConnectionSettings{
		ConnectionIP: "127.0.0.1",
		Port:         ":1234",
	}
	builder := newConnectionBuilder()

	builder.useSettings(connectionSettings)
	if builder.err != nil {
		t.Fatalf("Expected no error, got %v", builder.err)
	}

	if builder.settings.ConnectionIP != connectionSettings.ConnectionIP {
		t.Fatalf("Expected connectionSettings.ConnectionIP to be %v, got %v", connectionSettings.ConnectionIP, builder.settings.ConnectionIP)
	}

	if builder.settings.Port != connectionSettings.Port {
		t.Fatalf("Expected connectionSettings.Port to be %v, got %v", connectionSettings.Port, builder.settings.Port)
	}
}

func TestConnectionBuilder_UseConnectionTimeout(t *testing.T) {
	timeout := 2 * time.Second
	builder := newConnectionBuilder()

	builder.useConnectionTimeout(timeout)
	if builder.dialTimeout != timeout {
		t.Fatalf("Expected timeout to be 2s, got %v", builder.dialTimeout)
	}
}

func TestConnectionBuilder_Connect(t *testing.T) {
	// Setup UDP mock-server
	addr := ":12345"
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatalf("Failed to resolve UDP address: %v", err)
	}
	server, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("Failed to start UDP server: %v", err)
	}
	defer func(server *net.UDPConn) {
		_ = server.Close()
	}(server)

	builder := newConnectionBuilder()
	builder.useSettings(settings.ConnectionSettings{
		ConnectionIP: "127.0.0.1",
		Port:         addr,
	})
	builder.connect(context.Background())

	if builder.err != nil {
		t.Fatalf("Expected no error, got %v", builder.err)
	}

	if builder.conn == nil {
		t.Fatal("Expected connection to be established, got nil")
	}
}

func TestConnectionBuilder_Build(t *testing.T) {
	builder := newConnectionBuilder()
	builder.useSettings(settings.ConnectionSettings{
		ConnectionIP: "127.0.0.1",
		Port:         ":1234",
	}).useConnectionTimeout(2 * time.Second)

	conn, session, err := builder.build()
	if err == nil {
		t.Fatal("Expected error due to incomplete setup, got nil")
	}

	if conn != nil || session != nil {
		t.Fatal("Expected nil connection and session due to error, got non-nil")
	}
}
