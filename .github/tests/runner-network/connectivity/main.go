package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Published only after both sockets are listening. The marker identifies a run attempt.
type endpoint struct {
	Addresses []string
	TCPPort   int
	UDPPort   int
	Marker    string
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if len(os.Args) < 4 {
		log.Fatal("usage: connectivity server endpoint.json marker ip... | connectivity client endpoint.json marker")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var err error
	switch os.Args[1] {
	case "server":
		err = serve(ctx, os.Args[2], os.Args[3], os.Args[4:])
	case "client":
		err = checkServer(os.Args[2], os.Args[3])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}

func serve(ctx context.Context, path, marker string, addresses []string) error {
	if len(addresses) == 0 || marker == "" {
		return errors.New("server requires advertised addresses and a run marker")
	}
	for _, address := range addresses {
		if ip := net.ParseIP(address); ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid advertised IPv4 address %q", address)
		}
	}
	tcp, err := net.Listen("tcp4", ":0")
	if err != nil {
		return err
	}
	defer func() { _ = tcp.Close() }()
	udp, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return err
	}
	defer func() { _ = udp.Close() }()
	data, err := json.Marshal(endpoint{
		Addresses: addresses, Marker: marker,
		TCPPort: tcp.Addr().(*net.TCPAddr).Port,
		UDPPort: udp.LocalAddr().(*net.UDPAddr).Port,
	})
	if err != nil {
		return err
	}
	// A reader must never observe a partially written readiness file.
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return err
	}
	log.Printf("READY %s", data)
	stop := context.AfterFunc(ctx, func() {
		_ = tcp.Close()
		_ = udp.Close()
	})
	defer stop()
	results := make(chan error, 2)
	go func() { results <- serveTCP(tcp, marker) }()
	go func() { results <- serveUDP(udp, marker) }()
	err = <-results
	_ = tcp.Close()
	_ = udp.Close()
	<-results
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func serveTCP(listener net.Listener, marker string) error {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return err
		}
		if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			_ = connection.Close()
			return err
		}
		message, err := bufio.NewReader(io.LimitReader(connection, 256)).ReadString('\n')
		if err == nil && strings.HasPrefix(message, marker+" ") {
			_, err = io.WriteString(connection, message)
		}
		log.Printf("TCP peer=%s request=%q error=%v", connection.RemoteAddr(), message, err)
		_ = connection.Close()
	}
}

func serveUDP(connection net.PacketConn, marker string) error {
	buffer := make([]byte, 256)
	for {
		n, address, err := connection.ReadFrom(buffer)
		if err != nil {
			return err
		}
		message := string(buffer[:n])
		if strings.HasPrefix(message, marker+" ") && strings.HasSuffix(message, "\n") {
			_, err = connection.WriteTo(buffer[:n], address)
		}
		log.Printf("UDP peer=%s request=%q error=%v", address, message, err)
	}
}

func checkServer(path, marker string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var server endpoint
	if err := json.Unmarshal(data, &server); err != nil {
		return err
	}
	if server.Marker != marker || marker == "" {
		return errors.New("server belongs to a different run attempt")
	}
	message := marker + " " + runtime.GOOS + "/" + runtime.GOARCH + "\n"
	var failures []error
	for _, transport := range []struct {
		name string
		port int
	}{{"tcp4", server.TCPPort}, {"udp4", server.UDPPort}} {
		err := errors.New("no reachable server address")
		for _, ip := range server.Addresses {
			address := net.JoinHostPort(ip, strconv.Itoa(transport.port))
			for attempt := 1; attempt <= 3; attempt++ {
				err = exchange(transport.name, address, message)
				log.Printf("%s address=%s attempt=%d error=%v", transport.name, address, attempt, err)
				if err == nil {
					break
				}
			}
			if err == nil {
				log.Printf("PASS %s client=%s/%s server=%s marker=%s", transport.name, runtime.GOOS, runtime.GOARCH, address, marker)
				break
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", transport.name, err))
		}
	}
	return errors.Join(failures...)
}

func exchange(network, address, message string) error {
	connection, err := net.DialTimeout(network, address, 3*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	if _, err := io.WriteString(connection, message); err != nil {
		return err
	}
	response := make([]byte, len(message))
	if _, err := io.ReadFull(connection, response); err != nil {
		return err
	}
	if string(response) != message {
		return fmt.Errorf("unexpected response %q", response)
	}
	return nil
}
