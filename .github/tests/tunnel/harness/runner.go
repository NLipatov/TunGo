package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

func runFixture(ctx context.Context, args []string) error {
	r := &runner{}
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.StringVar(&r.protocol, "protocol", "", "UDP, TCP, or WS")
	flags.StringVar(&r.clientArch, "client-arch", "", "native runner architecture: amd64 or arm64")
	flags.StringVar(&r.serverArch, "server-arch", "", "Linux guest architecture: amd64 or arm64")
	flags.StringVar(&r.imageDir, "image-dir", "", "directory with kernel and initramfs.gz")
	flags.StringVar(&r.binary, "binary", "", "native TunGo client binary")
	flags.StringVar(&r.artifacts, "artifacts", "", "diagnostics directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return fmt.Errorf("this fixture changes host routes: run it only on disposable GitHub Actions runners")
	}
	if r.protocol != "UDP" && r.protocol != "TCP" && r.protocol != "WS" {
		return fmt.Errorf("invalid protocol %q", r.protocol)
	}
	if r.serverArch != "amd64" && r.serverArch != "arm64" {
		return fmt.Errorf("invalid server architecture %q", r.serverArch)
	}
	if r.clientArch != runtime.GOARCH {
		return fmt.Errorf("client binary architecture %s != %s", runtime.GOARCH, r.clientArch)
	}
	var hostArch string
	var err error
	if runtime.GOOS == "windows" {
		hostArch, err = powershell(ctx, "[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()")
	} else {
		hostArch, err = command(ctx, "uname", "-m")
	}
	if err != nil {
		return err
	}
	nativeArch := map[string]string{"x86_64": "amd64", "x64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[strings.ToLower(hostArch)]
	if nativeArch != r.clientArch {
		return fmt.Errorf("runner architecture %s != %s", hostArch, r.clientArch)
	}
	if r.imageDir == "" || r.binary == "" || r.artifacts == "" {
		return fmt.Errorf("image-dir, binary, and artifacts are required")
	}
	if err := prepareRunner(); err != nil {
		return err
	}
	if err := os.MkdirAll(r.artifacts, 0755); err != nil {
		return err
	}
	r.binary, err = filepath.Abs(r.binary)
	if err != nil {
		return err
	}
	configPath, endpoint := "/etc/tungo/client_configuration.json", "192.168.250.15"
	r.controlURL = "http://192.168.250.15:18080"
	if runtime.GOOS == "windows" {
		programData := os.Getenv("ProgramData")
		if programData == "" {
			programData = "C:/ProgramData"
		}
		configPath = filepath.Join(programData, "TunGo", "client_configuration.json")
		endpoint, r.controlURL = "127.77.0.1", "http://127.0.0.1:18080"
	}
	if _, err := os.Lstat(configPath); err == nil {
		return fmt.Errorf("refusing to overwrite existing TunGo configuration")
	} else if !os.IsNotExist(err) {
		return err
	}
	return r.run(ctx, configPath, endpoint)
}

type runner struct {
	protocol, clientArch, serverArch string
	imageDir, binary, artifacts      string
	controlURL                       string
	vm, client                       *child
	routes                           runnerRoutes
	config, tap                      string
}

func (r *runner) run(ctx context.Context, configPath, endpoint string) (err error) {
	defer func() {
		if err != nil {
			r.reportFailure()
		}
		err = errors.Join(err, r.cleanup())
	}()
	if err := r.boot(ctx); err != nil {
		return err
	}
	var info struct{ Arch, SHA256 string }
	if err := waitUntil(ctx, "Linux guest boot", 240*time.Second, func(ctx context.Context) error {
		if !r.vm.running() {
			return fmt.Errorf("VM exited: %v", r.vm.err)
		}
		return r.control(ctx, "/status", nil, &info)
	}); err != nil {
		return err
	}
	guestArch := "x86_64"
	if r.serverArch == "arm64" {
		guestArch = "aarch64"
	}
	if info.Arch != guestArch {
		return fmt.Errorf("guest architecture %s != %s", info.Arch, guestArch)
	}
	fmt.Printf("Native %s/%s -> Linux/%s (QEMU TCG), %s\n", runtime.GOOS, runtime.GOARCH, info.Arch, r.protocol)
	if err := r.routes.install(ctx); err != nil {
		return err
	}
	baseline, err := r.saveState(ctx, "client-before")
	if err != nil {
		return err
	}
	if err := r.noBypass(ctx); err != nil {
		return err
	}
	var config json.RawMessage
	if err := r.control(ctx, "/start", map[string]string{"protocol": r.protocol, "host": endpoint}, &config); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	r.config = configPath
	_, writeErr := file.Write(config)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	for cycle := 1; cycle <= 2; cycle++ {
		fmt.Printf("Connection cycle %d\n", cycle)
		r.client, err = startChild(filepath.Join(r.artifacts, fmt.Sprintf("client-%d.log", cycle)), nil, r.binary, "c")
		if err != nil {
			return err
		}
		for _, f := range families {
			if err := r.traffic(ctx, f, info.SHA256); err != nil {
				return err
			}
		}
		if _, err := r.saveState(ctx, fmt.Sprintf("client-connected-%d", cycle)); err != nil {
			return err
		}
		if err := r.client.stop(); err != nil {
			return err
		}
		r.client = nil
		if err := waitUntil(ctx, "client routes and interfaces restored", 30*time.Second, func(ctx context.Context) error {
			after, err := snapshot(ctx)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(after, baseline) {
				return fmt.Errorf("before=%v, after=%v", baseline, after)
			}
			return nil
		}); err != nil {
			return err
		}
		if _, err := r.saveState(ctx, fmt.Sprintf("client-after-%d", cycle)); err != nil {
			return err
		}
		if err := r.noBypass(ctx); err != nil {
			return err
		}
	}
	var stopped struct{ Restored bool }
	if err := r.control(ctx, "/stop", struct{}{}, &stopped); err != nil {
		return err
	}
	if !stopped.Restored {
		return fmt.Errorf("server did not confirm cleanup")
	}
	if err := r.noBypass(ctx); err != nil {
		return err
	}
	fmt.Println("PASS: dual-stack IPv4/IPv6 TUN, routes, 4 MiB checksum, NAT, client restart, graceful cleanup")
	return nil
}

func (r *runner) boot(ctx context.Context) error {
	qemuArch, machine, cpu, console := "x86_64", "q35", "max", "ttyS0"
	if r.serverArch == "arm64" {
		qemuArch, machine, cpu, console = "aarch64", "virt", "cortex-a72", "ttyAMA0"
	}
	qemu, err := exec.LookPath("qemu-system-" + qemuArch)
	if err != nil {
		return err
	}
	args := []string{qemu, "-accel", "tcg", "-m", "1024", "-smp", "2", "-display", "none",
		"-monitor", "none", "-no-reboot", "-kernel", filepath.Join(r.imageDir, "kernel"),
		"-initrd", filepath.Join(r.imageDir, "initramfs.gz"), "-serial", "stdio",
		"-append", "console=" + console + " rdinit=/init panic=1", "-machine", machine, "-cpu", cpu}
	var network string
	switch runtime.GOOS {
	case "darwin":
		network = "vmnet-host,id=transport,start-address=192.168.250.1,end-address=192.168.250.254,subnet-mask=255.255.255.0"
	case "linux":
		if _, err := command(ctx, "ip", "tuntap", "add", "dev", "tungo-vm0", "mode", "tap"); err != nil {
			return err
		}
		r.tap = "tungo-vm0"
		if _, err := command(ctx, "ip", "addr", "add", "192.168.250.1/24", "dev", r.tap); err != nil {
			return err
		}
		if _, err := command(ctx, "ip", "link", "set", r.tap, "up"); err != nil {
			return err
		}
		network = "tap,id=transport,ifname=" + r.tap + ",script=no,downscript=no"
	case "windows":
		// libslirp restrict=on drops UDP hostfwd replies. The guest firewall
		// instead permits replies and rejects new egress on the transport NIC.
		network = "user,id=transport,net=192.168.250.0/24," +
			"hostfwd=tcp:127.0.0.1:18080-192.168.250.15:18080," +
			"hostfwd=tcp:127.77.0.1:8080-192.168.250.15:8080," +
			"hostfwd=udp:127.77.0.1:9090-192.168.250.15:9090," +
			"hostfwd=tcp:127.77.0.1:1010-192.168.250.15:1010"
	}
	args = append(args, "-netdev", network, "-device", "virtio-net-pci,netdev=transport")
	r.vm, err = startChild(filepath.Join(r.artifacts, "vm.log"), nil, args...)
	return err
}

func (r *runner) noBypass(ctx context.Context) error {
	var status json.RawMessage
	if err := r.control(ctx, "/status", nil, &status); err != nil {
		return err
	}
	for _, f := range families {
		if _, err := request(ctx, f.url("/sha256"), nil, 2*time.Second); err == nil {
			return fmt.Errorf("IPv%d target reachable without TunGo", f.number)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (r *runner) traffic(ctx context.Context, f family, expectedHash string) error {
	fmt.Printf("Checking IPv%d traffic\n", f.number)
	if err := waitUntil(ctx, fmt.Sprintf("tunneled IPv%d HTTP", f.number), 90*time.Second, func(ctx context.Context) error {
		if !r.client.running() {
			return fmt.Errorf("client exited: %v", r.client.err)
		}
		_, err := request(ctx, f.url("/sha256"), nil, 2*time.Second)
		return err
	}); err != nil {
		return err
	}
	if err := assertTunnelRoute(ctx, f); err != nil {
		return err
	}
	args := []string{"ping", fmt.Sprintf("-%d", f.number), "-n", "-c", "3", f.server}
	if runtime.GOOS == "windows" {
		args = []string{"ping", fmt.Sprintf("-%d", f.number), "-n", "3", "-w", "2000", f.server}
	} else if runtime.GOOS == "darwin" {
		name := "ping"
		if f.number == 6 {
			name = "ping6"
		}
		args = []string{name, "-n", "-c", "3", f.server}
	}
	if _, err := command(ctx, args...); err != nil {
		return err
	}
	body, err := request(ctx, f.url("/payload"), nil, 60*time.Second)
	if err != nil {
		return err
	}
	if len(body) != fourMiB || fmt.Sprintf("%x", sha256.Sum256(body)) != expectedHash {
		return fmt.Errorf("IPv%d payload size or checksum mismatch", f.number)
	}
	peer, err := request(ctx, f.url("/peer"), nil, 10*time.Second)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(peer)) != f.nat {
		return fmt.Errorf("IPv%d MASQUERADE missing: peer=%s", f.number, peer)
	}
	return nil
}

func (r *runner) reportFailure() {
	ctx := context.Background()
	if _, err := r.saveState(ctx, "client-failure"); err != nil {
		fmt.Fprintln(os.Stderr, "Client snapshot:", err)
	}
	if r.vm != nil && r.vm.running() {
		if err := r.diagnostics(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "Server diagnostics:", err)
		}
	}
	for _, pattern := range []string{"*.log", "client-failure.json", "server.json"} {
		paths, _ := filepath.Glob(filepath.Join(r.artifacts, pattern))
		for _, path := range paths {
			data, _ := os.ReadFile(path)
			if len(data) > 20000 {
				data = data[len(data)-20000:]
			}
			fmt.Fprintf(os.Stderr, "%s:\n%s\n", filepath.Base(path), data)
		}
	}
}

func (r *runner) cleanup() error {
	ctx := context.Background()
	var errs []error
	if r.client != nil && r.client.running() {
		errs = append(errs, r.client.stop())
		r.client.kill()
	}
	if _, err := r.saveState(ctx, "client-final"); err != nil {
		fmt.Fprintln(os.Stderr, "Final snapshot:", err)
	}
	if r.vm != nil && r.vm.running() {
		if err := r.diagnostics(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "Final server diagnostics:", err)
		}
		_ = signalChild(r.vm.cmd.Process)
		if err := r.vm.wait(10 * time.Second); err != nil {
			r.vm.kill()
		}
	}
	if r.config != "" {
		errs = append(errs, os.Remove(r.config))
	}
	errs = append(errs, r.routes.remove(ctx))
	if r.tap != "" {
		_, err := command(ctx, "ip", "link", "delete", r.tap)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (r *runner) saveState(ctx context.Context, name string) (map[string][]string, error) {
	state, err := snapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	return state, os.WriteFile(filepath.Join(r.artifacts, name+".json"), data, 0644)
}

func (r *runner) diagnostics(ctx context.Context) error {
	var data json.RawMessage
	if err := r.control(ctx, "/diagnostics", nil, &data); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.artifacts, "server.json"), data, 0644)
}

func (r *runner) control(ctx context.Context, path string, data, result any) error {
	body, err := request(ctx, r.controlURL+path, data, 45*time.Second)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

func waitUntil(ctx context.Context, description string, timeout time.Duration, action func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last error
	for {
		if last = action(ctx); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w; last error: %v", description, ctx.Err(), last)
		case <-time.After(500 * time.Millisecond):
		}
	}
}
