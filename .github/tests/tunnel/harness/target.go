package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

func runTarget(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("target", flag.ContinueOnError)
	bind := flags.String("bind", "0.0.0.0", "HTTP target address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	network := "tcp4"
	if strings.Contains(*bind, ":") {
		network = "tcp6"
	}
	return serve(ctx, network, net.JoinHostPort(*bind, "8080"), newTarget())
}

type target struct {
	sum     []byte
	payload []byte
}

func newTarget() *target {
	payload := make([]byte, fourMiB)
	for i := range payload {
		payload[i] = byte(i)
	}
	return &target{
		sum:     []byte(fmt.Sprintf("%x\n", sha256.Sum256(payload))),
		payload: payload,
	}
}

func (t *target) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body []byte
	w.Header().Set("Content-Type", "text/plain")
	switch r.URL.Path {
	case "/payload":
		body = t.payload
		w.Header().Set("Content-Type", "application/octet-stream")
	case "/sha256":
		body = t.sum
	case "/peer":
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body = []byte(peer + "\n")
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	_, _ = w.Write(body)
}
