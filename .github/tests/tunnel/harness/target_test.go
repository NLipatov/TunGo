package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTargetTraffic(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(newTarget())
			server.Listener.Close()
			server.Listener = listener
			server.Start()
			defer server.Close()

			body, err := request(context.Background(), server.URL+"/payload", nil, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) != 4*1024*1024 {
				t.Fatalf("download truncated: %d bytes", len(body))
			}
			for i, value := range body {
				if value != byte(i) {
					t.Fatalf("corrupt payload at byte %d: %d", i, value)
				}
			}
			hash, err := request(context.Background(), server.URL+"/sha256", nil, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("%x\n", sha256.Sum256(body)); string(hash) != want {
				t.Fatalf("checksum %q != %q", hash, want)
			}
			peer, err := request(context.Background(), server.URL+"/peer", nil, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if string(peer) != host+"\n" {
				t.Fatalf("peer %q != %q", peer, host)
			}
		})
	}
}

func TestRequestControl(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected control request: %s %v", r.Method, r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"protocol":"UDP"}` {
			t.Errorf("unexpected body: %s", body)
		}
		http.Error(w, "server not started", http.StatusInternalServerError)
	}))
	defer server.Close()
	_, err := request(context.Background(), server.URL+"/start", map[string]string{"protocol": "UDP"}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500: server not started") {
		t.Fatalf("controller error lost: %v", err)
	}
}
