package client_factory

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

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

// mkWSSettings returns minimal WS/WSS settings.
func mkWSSettings(host, ip, port string, proto settings.Protocol) settings.Settings {
	return settings.Settings{
		Host:          host,
		ConnectionIP:  ip,
		Port:          port,
		Protocol:      proto,
		DialTimeoutMs: 200,
	}
}

// ConnectionFactoryMockWSServer spins up a barebones WS echo server at /ws.
func ConnectionFactoryMockWSServer(t *testing.T) (host string, port string, shutdown func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	h, p, _ := strings.Cut(addr, ":")

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func(c *websocket.Conn, code websocket.StatusCode, reason string) {
			_ = c.Close(code, reason)
		}(c, websocket.StatusNormalClosure, "")
		// simple echo loop
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			_ = c.Write(r.Context(), typ, data)
		}
	})
	srv := &http.Server{Handler: mux}

	go func() {
		_ = srv.Serve(ln)
	}()

	return h, p, func() {
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
	}
}

// ---- tests ----

func Test_connectionSettings_TCP(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func Test_connectionSettings_WS(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("example.org", "", "80", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.WS || got.Port != "80" || got.Host != "example.org" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_WSS_UsesWSSettingsBucket(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("secure.example", "", "443", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.WSS || got.Port != "443" || got.Host != "secure.example" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_Unsupported(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{Protocol: 999}
	f := &ConnectionFactory{conf: conf}
	_, err := f.connectionSettings()
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

func TestEstablishConnection_InvalidPort_TCP_ParseError(t *testing.T) {
	t.Parallel()
	// Using "abc" as port will cause ParseAddrPort to fail.
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("abc"),
	}
	f := &ConnectionFactory{conf: conf}

	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected parse error for bad port")
	}
}

func TestEstablishConnection_InvalidPort_UDP_ParseError(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:    settings.UDP,
		UDPSettings: mkUDPSettings("bad"),
	}
	f := &ConnectionFactory{conf: conf}

	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected parse error for bad UDP port")
	}
}

func TestDialTCP_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestDialWS_Success(t *testing.T) {
	t.Parallel()
	host, port, shutdown := ConnectionFactoryMockWSServer(t)
	defer shutdown()

	f := &ConnectionFactory{}
	adapter, err := f.dialWS(context.Background(), context.Background(), "ws", host, port)
	if err != nil {
		t.Fatalf("dialWS failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil")
	}
	_ = adapter.Close()
}

func TestDialWS_Error_NoServer(t *testing.T) {
	t.Parallel()
	f := &ConnectionFactory{}
	// Use a port with no WS server
	adapter, err := f.dialWS(context.Background(), context.Background(), "ws", "127.0.0.1", "1")
	if err == nil {
		_ = adapter.Close()
		t.Fatalf("expected error when no WS server is listening")
	}
}

func TestEstablishConnection_WS_EmptyHost_And_EmptyConnectionIP(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("", "", "8080", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: empty host") {
		t.Fatalf("expected empty host error, got: %v", err)
	}
}

func TestEstablishConnection_WS_EmptyPort(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: empty port") {
		t.Fatalf("expected empty port error, got: %v", err)
	}
}

func TestEstablishConnection_WS_InvalidPort_NonNumeric(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "abc", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: invalid port") {
		t.Fatalf("expected invalid port error, got: %v", err)
	}
}

func TestEstablishConnection_WS_InvalidPort_OutOfRange(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "70000", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: invalid port") {
		t.Fatalf("expected invalid port error, got: %v", err)
	}
}

func TestEstablishConnection_WSS_EmptyHost(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("", "", "443", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "wss dial: empty host") {
		t.Fatalf("expected empty host error, got: %v", err)
	}
}

func TestEstablishConnection_WSS_DefaultPort443_And_WrappedError(t *testing.T) {
	t.Parallel()
	// No port -> defaults to 443; since nothing listens, expect wrapped WS dial error.
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("127.0.0.1", "", "", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unable to establish WebSocket connection") {
		t.Fatalf("expected wrapped WS connect error, got: %v", err)
	}
}

func TestEstablishConnection_WSS_InvalidPort_OutOfRange(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("127.0.0.1", "", "70000", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "wss dial: invalid port") {
		t.Fatalf("expected invalid port error, got: %v", err)
	}
}

func TestEstablishConnection_UnsupportedProtocol(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{Protocol: 999}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

// Verifies the error wrapping path for TCP.
func TestEstablishConnection_TCP_DialError_IsWrapped(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("1"), // likely closed → Dial error
	}
	f := &ConnectionFactory{conf: conf}

	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected dial error")
	}
	if !strings.Contains(err.Error(), "unable to establish TCP connection") {
		t.Fatalf("expected wrapped TCP dial error, got: %v", err)
	}
}

// Verifies the error wrapping path for WS with provided Host fallback.
func TestEstablishConnection_WS_DialError_IsWrapped(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "9", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unable to establish WebSocket connection") {
		t.Fatalf("expected wrapped WS dial error, got: %v", err)
	}
}
