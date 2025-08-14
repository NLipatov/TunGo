package client_factory

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

// mkTCPSettings returns minimal TCP settings for a given port.
func mkTCPSettings(port string) settings.Settings {
	return settings.Settings{
		ConnectionIP:  "127.0.0.1",
		Port:          port,
		Protocol:      settings.TCP,
		DialTimeoutMs: 100,
	}
}

// mkUDPSettings returns minimal UDP settings for a given port.
func mkUDPSettings(port string) settings.Settings {
	return settings.Settings{
		ConnectionIP:  "127.0.0.1",
		Port:          port,
		Protocol:      settings.UDP,
		DialTimeoutMs: 100,
	}
}

func Test_connectionSettings_TCP(t *testing.T) {
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("443"),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.TCP || got.Port != "443" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_UDP(t *testing.T) {
	conf := client.Configuration{
		Protocol:    settings.UDP,
		UDPSettings: mkUDPSettings("53"),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.UDP || got.Port != "53" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_Unsupported(t *testing.T) {
	conf := client.Configuration{
		Protocol: 999, // unsupported
	}
	f := &ConnectionFactory{conf: conf}
	_, err := f.connectionSettings()
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

func TestEstablishConnection_InvalidPort_ParseError(t *testing.T) {
	// Using "abc" as port will cause ParseAddrPort to fail.
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("abc"),
	}
	f := &ConnectionFactory{conf: conf}

	_, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected parse error for bad port")
	}
}

func TestDialTCP_Success(t *testing.T) {
	// Start a temporary TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen tcp: %v", err)
	}
	defer func(ln net.Listener) { _ = ln.Close() }(ln)

	// Accept one connection in the background
	done := make(chan struct{})
	go func() {
		defer close(done)
		if conn, err := ln.Accept(); err == nil {
			_ = conn.Close()
		}
	}()

	ap := netip.MustParseAddrPort(ln.Addr().String())
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	adapter, err := f.dialTCP(ctx, ap)
	if err != nil {
		t.Fatalf("dialTCP failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil on success")
	}
	_ = adapter.Close()
	<-done
}

func TestDialTCP_Refused(t *testing.T) {
	// Port 1 on localhost is almost always closed -> should return an error quickly.
	ap := netip.MustParseAddrPort("127.0.0.1:1")
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	adapter, err := f.dialTCP(ctx, ap)
	if err == nil {
		_ = adapter.Close()
		t.Fatalf("expected error dialing to closed port")
	}
}

func TestDialUDP_Success_NoServerNeeded(t *testing.T) {
	// UDP "dial" does not require a server to succeed in most cases.
	ap := netip.MustParseAddrPort("127.0.0.1:9") // discard port
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := f.dialUDP(ctx, ap)
	if err != nil {
		t.Fatalf("dialUDP failed: %v", err)
	}
	if conn == nil {
		t.Fatalf("conn must not be nil")
	}
	_ = conn.Close()
}

func TestEstablishConnection_UnsupportedProtocol(t *testing.T) {
	conf := client.Configuration{Protocol: 999}
	f := &ConnectionFactory{conf: conf}
	_, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

// NEW: full path error on TCP dial (checks error wrapping message branch)
func TestEstablishConnection_TCP_DialError_IsWrapped(t *testing.T) {
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("1"), // likely closed → Dial error
	}
	f := &ConnectionFactory{conf: conf}

	_, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected dial error")
	}
	// Optional: verify our wrapping message prefix
	if !strings.Contains(err.Error(), "unable to establish TCP connection") {
		t.Fatalf("expected wrapped TCP dial error, got: %v", err)
	}
}
