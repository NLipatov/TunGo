package client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/net/http/httpproxy"
)

func TestDialWebSocketEndpoint_ProxyTLS(t *testing.T) {
	for _, tc := range []struct{ name, scheme, host string }{
		{"http", "http", "localhost"},
		{"https", "https", "localhost"},
		{"socks5", "socks5", "localhost"},
		{"socks5h", "socks5h", "localhost"},
		{"https_idn", "https", "ℓocalhost"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := tc.scheme
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			originHello := make(chan *tls.ClientHelloInfo, 1)
			originCert, originRoot := proxyTestCertificate(t, "origin.test")
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Proxy-Authorization") != "" {
					t.Error("proxy credentials reached the origin")
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = conn.CloseNow() }()
				for {
					kind, payload, err := conn.Read(ctx)
					if err != nil {
						return
					}
					if err := conn.Write(ctx, kind, payload); err != nil {
						return
					}
				}
			}))
			origin.TLS = &tls.Config{
				Certificates: []tls.Certificate{originCert},
				GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
					originHello <- hello
					return nil, nil
				},
			}
			origin.StartTLS()
			t.Cleanup(origin.Close)

			proxyHello := make(chan *tls.ClientHelloInfo, 1)
			roots := x509.NewCertPool()
			roots.AddCert(originRoot)
			var proxyURL *url.URL
			if scheme == "socks5" || scheme == "socks5h" {
				proxyURL = proxyTestSOCKS(t, ctx, origin.Listener.Addr().String())
				proxyURL.Scheme = scheme
			} else {
				proxyCert, proxyRoot := proxyTestCertificate(t, "localhost")
				proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodConnect || r.Host != "origin.test:443" {
						t.Errorf("proxy request = %s %s", r.Method, r.Host)
						http.Error(w, "unexpected target", http.StatusBadRequest)
						return
					}
					wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:password"))
					if r.Header.Get("Proxy-Authorization") != wantAuth {
						t.Error("missing proxy authentication")
						http.Error(w, "authentication required", http.StatusProxyAuthRequired)
						return
					}
					proxyTestRelay(t, ctx, w, origin.Listener.Addr().String())
				}))
				if scheme == "https" {
					proxy.TLS = &tls.Config{
						Certificates: []tls.Certificate{proxyCert},
						GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
							proxyHello <- hello
							return nil, nil
						},
					}
					proxy.StartTLS()
				} else {
					proxy.Start()
				}
				t.Cleanup(proxy.Close)
				proxyURL = &url.URL{
					Scheme: scheme,
					Host:   net.JoinHostPort(tc.host, strconv.Itoa(proxy.Listener.Addr().(*net.TCPAddr).Port)),
					User:   url.UserPassword("user", "password"),
				}
				roots.AddCert(proxyRoot)
			}
			proxyTestTransport(t, proxyURL, roots)

			establishCtx, establishCancel := context.WithCancel(ctx)
			defer establishCancel()
			conn, err := dialWebSocketEndpoint(establishCtx, ctx, "wss", "origin.test:443")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			establishCancel() // The established tunnel must outlive the dial context.
			remote, ok := conn.(interface{ RemoteAddrPort() netip.AddrPort })
			if !ok || !remote.RemoteAddrPort().Addr().IsLoopback() || strconv.Itoa(int(remote.RemoteAddrPort().Port())) != proxyURL.Port() {
				t.Fatalf("remote address must identify the proxy, got %v", remote)
			}
			payload := []byte("framed WebSocket through a proxy")
			if _, err := conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("echo = %q, want %q", got, payload)
			}
			assertProxyTestHello(t, <-originHello, "origin.test")
			if scheme == "https" {
				assertProxyTestHello(t, <-proxyHello, "localhost")
			}
		})
	}
}

func proxyTestCertificate(t *testing.T, name string) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: certificate}, certificate
}

func proxyTestTransport(t *testing.T, proxyURL *url.URL, roots *x509.CertPool) {
	t.Helper()
	previous := http.DefaultTransport
	transport := cloneDefaultTransport()
	transport.Proxy = http.ProxyURL(proxyURL)
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = previous
		transport.CloseIdleConnections()
	})
}

func proxyTestRelay(t *testing.T, ctx context.Context, w http.ResponseWriter, target string) {
	t.Helper()
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		t.Error(err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer func() { _ = upstream.Close() }()
	downstream, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() { _ = downstream.Close() }()
	t.Cleanup(func() {
		_ = downstream.Close()
		_ = upstream.Close()
	})
	if _, err := io.WriteString(downstream, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		t.Error(err)
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, downstream)
		_ = upstream.Close()
		close(done)
	}()
	_, _ = io.Copy(downstream, upstream)
	_ = downstream.Close()
	<-done
}

func assertProxyTestHello(t *testing.T, hello *tls.ClientHelloInfo, serverName string) {
	t.Helper()
	if hello.ServerName != serverName {
		t.Errorf("SNI = %q, want %q", hello.ServerName, serverName)
	}
	if !slices.Equal(hello.SupportedProtos, []string{"http/1.1"}) {
		t.Errorf("%s ALPN = %v, want http/1.1", serverName, hello.SupportedProtos)
	}
	isGREASE := func(value uint16) bool {
		return value&0x0f0f == 0x0a0a && byte(value>>8) == byte(value)
	}
	if !slices.ContainsFunc(hello.CipherSuites, isGREASE) || !slices.ContainsFunc(hello.Extensions, isGREASE) {
		t.Errorf("%s ClientHello lost the browser preset's GREASE", serverName)
	}
	if slices.Contains(hello.Extensions, 17513) || slices.Contains(hello.Extensions, 17613) {
		t.Errorf("%s HTTP/1.1 ClientHello advertises ALPS", serverName)
	}
}

func proxyTestSOCKS(t *testing.T, ctx context.Context, target string) *url.URL {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	t.Cleanup(func() {
		_ = listener.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	go func() {
		done <- func() error {
			conn, err := listener.Accept()
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()
			deadline, _ := ctx.Deadline()
			_ = conn.SetDeadline(deadline)
			read := func(n int) ([]byte, error) {
				data := make([]byte, n)
				_, err := io.ReadFull(conn, data)
				return data, err
			}
			greeting, err := read(2)
			if err != nil {
				return err
			}
			methods, err := read(int(greeting[1]))
			if err != nil {
				return err
			}
			if greeting[0] != 5 || !slices.Contains(methods, byte(2)) {
				return errors.New("SOCKS5 username/password authentication was not offered")
			}
			if _, err := conn.Write([]byte{5, 2}); err != nil {
				return err
			}
			auth, err := read(2)
			if err != nil {
				return err
			}
			user, err := read(int(auth[1]))
			if err != nil {
				return err
			}
			length, err := read(1)
			if err != nil {
				return err
			}
			password, err := read(int(length[0]))
			if err != nil {
				return err
			}
			if auth[0] != 1 || string(user) != "user" || string(password) != "password" {
				return errors.New("incorrect SOCKS5 credentials")
			}
			if _, err := conn.Write([]byte{1, 0}); err != nil {
				return err
			}
			request, err := read(5)
			if err != nil {
				return err
			}
			if !bytes.Equal(request[:4], []byte{5, 1, 0, 3}) {
				return fmt.Errorf("expected SOCKS CONNECT with remote DNS, got %v", request)
			}
			address, err := read(int(request[4]) + 2)
			if err != nil {
				return err
			}
			if string(address[:len(address)-2]) != "origin.test" || binary.BigEndian.Uint16(address[len(address)-2:]) != 443 {
				return fmt.Errorf("unexpected SOCKS target: %v", address)
			}
			upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
			if err != nil {
				return err
			}
			defer func() { _ = upstream.Close() }()
			if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
				return err
			}
			copied := make(chan struct{})
			go func() {
				_, _ = io.Copy(upstream, conn)
				_ = upstream.Close()
				close(copied)
			}()
			_, _ = io.Copy(conn, upstream)
			_ = conn.Close()
			<-copied
			return nil
		}()
	}()
	return &url.URL{Scheme: "socks5", Host: listener.Addr().String(), User: url.UserPassword("user", "password")}
}

func TestDialWebSocketEndpoint_ProxyCancellation(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			closed := make(chan error, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					closed <- err
					return
				}
				defer func() { _ = conn.Close() }()
				_ = conn.SetReadDeadline(time.Now().Add(time.Second))
				cancel() // Cancel while the client is waiting for a CONNECT response.
				var b [1]byte
				_, err = conn.Read(b[:])
				closed <- err
			}))
			cert, root := proxyTestCertificate(t, "localhost")
			if scheme == "https" {
				server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
				server.StartTLS()
			} else {
				server.Start()
			}
			t.Cleanup(server.Close)
			proxyURL := &url.URL{Scheme: scheme, Host: net.JoinHostPort("localhost", strconv.Itoa(server.Listener.Addr().(*net.TCPAddr).Port))}
			roots := x509.NewCertPool()
			roots.AddCert(root)
			proxyTestTransport(t, proxyURL, roots)
			conn, err := dialWebSocketEndpoint(ctx, t.Context(), "wss", "origin.test:443")
			if conn != nil {
				_ = conn.Close()
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("dial error = %v, want context.Canceled", err)
			}
			if err := <-closed; !errors.Is(err, io.EOF) {
				t.Fatalf("proxy read = %v, want EOF after cancellation", err)
			}
		})
	}
}

func TestDialWebSocketEndpoint_ProxyRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "authentication required", http.StatusProxyAuthRequired)
	}))
	t.Cleanup(server.Close)
	proxyURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyTestTransport(t, proxyURL, nil)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	conn, err := dialWebSocketEndpoint(ctx, ctx, "wss", "origin.test:443")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "407") {
		t.Fatalf("dial error = %v, want proxy rejection", err)
	}
}

func TestDialWebSocketEndpoint_ProxyCertificates(t *testing.T) {
	for _, hop := range []string{"proxy", "origin"} {
		for _, failure := range []string{"untrusted", "hostname"} {
			t.Run(hop+"/"+failure, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				originName, proxyName := "origin.test", "localhost"
				if failure == "hostname" {
					if hop == "origin" {
						originName = "wrong.test"
					} else {
						proxyName = "wrong.test"
					}
				}
				originCert, originRoot := proxyTestCertificate(t, originName)
				origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("WebSocket upgrade reached an unverified origin")
				}))
				origin.TLS = &tls.Config{Certificates: []tls.Certificate{originCert}}
				origin.StartTLS()
				t.Cleanup(origin.Close)
				var connected atomic.Bool
				proxyCert, proxyRoot := proxyTestCertificate(t, proxyName)
				proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					connected.Store(true)
					proxyTestRelay(t, ctx, w, origin.Listener.Addr().String())
				}))
				proxy.TLS = &tls.Config{Certificates: []tls.Certificate{proxyCert}}
				proxy.StartTLS()
				t.Cleanup(proxy.Close)
				proxyURL := &url.URL{Scheme: "https", Host: net.JoinHostPort("localhost", strconv.Itoa(proxy.Listener.Addr().(*net.TCPAddr).Port))}
				roots := x509.NewCertPool()
				if hop != "origin" || failure != "untrusted" {
					roots.AddCert(originRoot)
				}
				if hop != "proxy" || failure != "untrusted" {
					roots.AddCert(proxyRoot)
				}
				proxyTestTransport(t, proxyURL, roots)
				conn, err := dialWebSocketEndpoint(ctx, ctx, "wss", "origin.test:443")
				if conn != nil {
					_ = conn.Close()
				}
				if failure == "hostname" {
					var hostnameError x509.HostnameError
					if !errors.As(err, &hostnameError) {
						t.Fatalf("dial error = %v, want hostname verification failure", err)
					}
				} else {
					var authorityError x509.UnknownAuthorityError
					if !errors.As(err, &authorityError) {
						t.Fatalf("dial error = %v, want untrusted certificate failure", err)
					}
				}
				if connected.Load() != (hop == "origin") {
					t.Errorf("CONNECT received = %v for invalid %s certificate", connected.Load(), hop)
				}
			})
		}
	}
}

func TestDialWebSocketEndpoint_ProxyBypass(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cert, root := proxyTestCertificate(t, "localhost")
	hellos := make(chan *tls.ClientHelloInfo, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, _, _ = conn.Read(ctx)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			hellos <- hello
			return nil, nil
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	proxyTestTransport(t, nil, roots)
	target := net.JoinHostPort("localhost", strconv.Itoa(server.Listener.Addr().(*net.TCPAddr).Port))
	var lookedUp atomic.Bool
	http.DefaultTransport.(*http.Transport).Proxy = func(r *http.Request) (*url.URL, error) {
		lookedUp.Store(true)
		if r.URL.Scheme != "https" || r.URL.Host != target {
			t.Errorf("proxy lookup URL = %s", r.URL)
		}
		return nil, nil // Same contract as a NO_PROXY match.
	}
	conn, err := dialWebSocketEndpoint(ctx, ctx, "wss", target)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if !lookedUp.Load() {
		t.Fatal("WSS ignored the configured proxy selector")
	}
	assertProxyTestHello(t, <-hellos, "localhost")
}

func TestDialWebSocketEndpoint_RedirectKeepsProxy(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			previousResolver := net.DefaultResolver
			var directDNS atomic.Bool
			net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				directDNS.Store(true)
				return nil, errors.New("direct DNS is forbidden by this test")
			}}
			t.Cleanup(func() { net.DefaultResolver = previousResolver })
			cert, root := proxyTestCertificate(t, "origin.test")
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "http://redirect.test/ws", http.StatusFound)
			}))
			origin.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
			origin.StartTLS()
			t.Cleanup(origin.Close)
			var redirected atomic.Bool
			forward := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodConnect {
					proxyTestRelay(t, ctx, w, origin.Listener.Addr().String())
					return
				}
				if r.Host != "redirect.test" {
					t.Errorf("unexpected redirect host %q", r.Host)
					http.Error(w, "unexpected target", http.StatusBadRequest)
					return
				}
				redirected.Store(true)
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = conn.CloseNow() }()
				_, _, _ = conn.Read(ctx)
			}))
			roots := x509.NewCertPool()
			roots.AddCert(root)
			hellos := make(chan *tls.ClientHelloInfo, 2)
			if scheme == "https" {
				proxyCert, proxyRoot := proxyTestCertificate(t, "localhost")
				roots.AddCert(proxyRoot)
				forward.TLS = &tls.Config{
					Certificates: []tls.Certificate{proxyCert},
					GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
						hellos <- hello
						return nil, nil
					},
				}
				forward.StartTLS()
			} else {
				forward.Start()
			}
			t.Cleanup(forward.Close)
			proxyURL := &url.URL{Scheme: scheme, Host: net.JoinHostPort("localhost", strconv.Itoa(forward.Listener.Addr().(*net.TCPAddr).Port))}
			proxyTestTransport(t, proxyURL, roots)
			t.Setenv("HTTP_PROXY", proxyURL.String())
			t.Setenv("HTTPS_PROXY", proxyURL.String())
			t.Setenv("NO_PROXY", "")
			t.Setenv("no_proxy", "")
			t.Setenv("REQUEST_METHOD", "")
			// Same environment policy as net/http, without its process-global cache.
			selectProxy := httpproxy.FromEnvironment().ProxyFunc()
			http.DefaultTransport.(*http.Transport).Proxy = func(r *http.Request) (*url.URL, error) {
				return selectProxy(r.URL)
			}
			conn, err := dialWebSocketEndpoint(ctx, ctx, "wss", "origin.test:443")
			if conn != nil {
				_ = conn.Close()
			}
			if err != nil || !redirected.Load() || directDNS.Load() {
				t.Fatalf("redirect reached proxy=%v; direct DNS=%v; dial error=%v", redirected.Load(), directDNS.Load(), err)
			}
			if scheme == "https" {
				// The initial CONNECT and redirected HTTP request use separate
				// proxy connections; both must retain the browser ClientHello.
				for range 2 {
					select {
					case hello := <-hellos:
						assertProxyTestHello(t, hello, "localhost")
					default:
						t.Fatal("missing TLS connection to the HTTPS proxy")
					}
				}
			}
		})
	}
}

func TestDialWebSocketEndpoint_IDNProxyLookup(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			previousResolver := net.DefaultResolver
			var lookups atomic.Int64
			net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				lookups.Add(1)
				return nil, errors.New("stop at DNS boundary")
			}}
			t.Cleanup(func() { net.DefaultResolver = previousResolver })
			proxyURL := &url.URL{Scheme: scheme, Host: "прокси.test:3128"}
			proxyTestTransport(t, proxyURL, nil)
			conn, err := dialWebSocketEndpoint(ctx, ctx, "wss", "origin.test:443")
			if conn != nil {
				_ = conn.Close()
			}
			var dnsErr *net.DNSError
			if !errors.As(err, &dnsErr) {
				t.Fatalf("expected diagnostic DNS failure, got %v", err)
			}
			if lookups.Load() == 0 || dnsErr.Name != "xn--h1adldfi.test" {
				t.Fatalf("DNS attempts=%d; hostname=%q, want an ASCII proxy lookup", lookups.Load(), dnsErr.Name)
			}
			if proxyURL.Host != "прокси.test:3128" {
				t.Fatal("dial mutated the shared proxy URL")
			}
		})
	}
}
