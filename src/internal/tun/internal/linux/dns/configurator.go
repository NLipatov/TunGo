package dns

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
)

type backend uint8

const (
	unknown backend = iota
	resolved
	resolvconf
	resolvconfKey = "tungo"
)

type Configurator struct {
	runner          runner
	activeInterface string
	activeBackend   backend
}

type runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error)
}

func New(runner runner) *Configurator {
	return &Configurator{runner: runner}
}

func (c *Configurator) Set(ifName string, ipv4Resolvers, ipv6Resolvers []string) error {
	selected, err := c.detectBackend()
	if err != nil {
		return err
	}
	return c.configure(selected, ifName, ipv4Resolvers, ipv6Resolvers)
}

func (c *Configurator) Revert() error {
	if c.activeInterface == "" {
		if err := c.removeResolvconfEntry(); err != nil && !errors.Is(err, exec.ErrNotFound) {
			return err
		}
		return nil
	}
	ifName := c.activeInterface

	var err error
	switch c.activeBackend {
	case resolved:
		err = c.run("resolvectl", "revert", ifName)
	case resolvconf:
		err = c.removeResolvconfEntry()
	}
	if err != nil {
		return fmt.Errorf("restore DNS for %s: %w", ifName, err)
	}
	c.activeInterface = ""
	c.activeBackend = unknown
	return nil
}

func (c *Configurator) removeResolvconfEntry() error {
	return c.run("resolvconf", "-d", resolvconfKey, "-f")
}

func (c *Configurator) configure(
	selected backend,
	ifName string,
	ipv4Resolvers, ipv6Resolvers []string,
) error {
	resolvers := slices.Concat(ipv4Resolvers, ipv6Resolvers)
	changed, applyErr := c.apply(selected, ifName, resolvers)
	if applyErr == nil {
		c.activeInterface = ifName
		c.activeBackend = selected
		return nil
	}
	if !changed {
		return applyErr
	}

	c.activeInterface = ifName
	c.activeBackend = selected
	if cleanupErr := c.Revert(); cleanupErr != nil {
		return errors.Join(applyErr, cleanupErr)
	}
	return applyErr
}

func (c *Configurator) detectBackend() (backend, error) {
	target, err := os.Readlink("/etc/resolv.conf")
	if err == nil {
		detected, detectErr := c.detectLinkBackend(target)
		if detectErr != nil {
			return unknown, detectErr
		}
		if detected != unknown {
			return detected, nil
		}
	}

	contents, readErr := os.ReadFile("/etc/resolv.conf")
	if readErr != nil {
		if err == nil {
			return unknown, fmt.Errorf("unsupported DNS owner: /etc/resolv.conf points to %q and cannot be read: %w", target, readErr)
		}
		return unknown, fmt.Errorf("inspect DNS owner: readlink /etc/resolv.conf: %v; read /etc/resolv.conf: %w", err, readErr)
	}
	detected, detectErr := c.detectStubBackend(string(contents))
	if detectErr != nil {
		return unknown, detectErr
	}
	if detected != unknown {
		return detected, nil
	}
	if err == nil {
		return unknown, fmt.Errorf("unsupported DNS owner: /etc/resolv.conf points to %q", target)
	}
	return unknown, errors.New("unsupported DNS owner: /etc/resolv.conf does not use systemd-resolved or resolvconf")
}

func (c *Configurator) detectLinkBackend(target string) (backend, error) {
	if strings.HasSuffix(target, "/systemd/resolve/stub-resolv.conf") ||
		strings.HasSuffix(target, "/systemd/resolve/resolv.conf") {
		if err := c.run("resolvectl", "status"); err != nil {
			return unknown, fmt.Errorf("/etc/resolv.conf points to systemd-resolved, but resolvectl is unavailable: %w", err)
		}
		return resolved, nil
	}
	if strings.Contains(target, "resolvconf/") {
		if err := c.run("resolvconf", "-l"); err != nil {
			return unknown, fmt.Errorf("/etc/resolv.conf points to resolvconf, but resolvconf is unavailable: %w", err)
		}
		return resolvconf, nil
	}
	return unknown, nil
}

func (c *Configurator) detectStubBackend(contents string) (backend, error) {
	if !usesResolvedStub(contents) {
		return unknown, nil
	}
	if err := c.run("resolvectl", "status"); err != nil {
		return unknown, fmt.Errorf("/etc/resolv.conf uses the systemd-resolved stub, but resolvectl is unavailable: %w", err)
	}
	return resolved, nil
}

func usesResolvedStub(contents string) bool {
	for line := range strings.SplitSeq(contents, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" && fields[1] == "127.0.0.53" {
			return true
		}
	}
	return false
}

func (c *Configurator) apply(
	selected backend,
	ifName string,
	resolvers []string,
) (bool, error) {
	switch selected {
	case resolved:
		return c.setResolved(ifName, resolvers)
	case resolvconf:
		return c.setResolvconf(resolvers)
	default:
		return false, fmt.Errorf("unsupported DNS backend %d", selected)
	}
}

func (c *Configurator) setResolved(ifName string, resolvers []string) (bool, error) {
	if err := c.run("resolvectl", "domain", ifName, "~."); err != nil {
		return false, fmt.Errorf("set DNS routing domain on %s: %w", ifName, err)
	}
	if err := c.run("resolvectl", "default-route", ifName, "true"); err != nil {
		return true, fmt.Errorf("make %s the default DNS route: %w", ifName, err)
	}
	args := append([]string{"dns", ifName}, resolvers...)
	if err := c.run("resolvectl", args...); err != nil {
		return true, fmt.Errorf("set DNS servers on %s: %w", ifName, err)
	}
	return true, nil
}

func (c *Configurator) setResolvconf(resolvers []string) (bool, error) {
	var input strings.Builder
	for _, resolver := range resolvers {
		fmt.Fprintf(&input, "nameserver %s\n", resolver)
	}
	output, err := c.runner.CombinedOutputWithInput(
		"resolvconf", strings.NewReader(input.String()),
		"-a", resolvconfKey, "-m", "0", "-x",
	)
	if err != nil {
		return !errors.Is(err, exec.ErrNotFound), commandError("resolvconf", output, err)
	}
	return true, nil
}

func (c *Configurator) run(name string, args ...string) error {
	output, err := c.runner.CombinedOutput(name, args...)
	if err != nil {
		return commandError(name, output, err)
	}
	return nil
}

func commandError(name string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s: %w: %s", name, err, detail)
}
