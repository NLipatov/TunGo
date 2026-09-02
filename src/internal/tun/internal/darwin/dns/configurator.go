//go:build darwin

package dns

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
)

const (
	globalIPv4Key             = "State:/Network/Global/IPv4"
	globalIPv6Key             = "State:/Network/Global/IPv6"
	backupKey                 = "State:/Network/TunGo/DNSBackup"
	serviceProperty           = "TunGoService"
	hadSetupInitiallyProperty = "TunGoHadSetupInitially"
)

type Configurator struct {
	runner runner
}

type runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error)
}

func New(runner runner) *Configurator {
	return &Configurator{runner: runner}
}

func (c *Configurator) Set(resolvers []string) error {
	if err := c.Revert(); err != nil {
		return fmt.Errorf("restore previous DNS: %w", err)
	}

	service, err := c.primaryService()
	if err != nil {
		return err
	}
	_, hadSetupInitially, err := c.show(setupKey(service))
	if err != nil {
		return fmt.Errorf("read DNS for %s: %w", service, err)
	}
	if err := c.backupDNS(service, hadSetupInitially); err != nil {
		return fmt.Errorf("save DNS backup for %s: %w", service, err)
	}

	if err := c.applyResolvers(service, resolvers); err != nil {
		setupErr := fmt.Errorf("set VPN DNS: %w", err)
		if cleanupErr := c.Revert(); cleanupErr != nil {
			return errors.Join(setupErr, cleanupErr)
		}
		return setupErr
	}

	c.flushCache()
	return nil
}

func (c *Configurator) Revert() error {
	rawBackup, exists, err := c.show(backupKey)
	if err != nil {
		return fmt.Errorf("read DNS backup: %w", err)
	}
	if !exists {
		return nil
	}

	service, hadSetupInitially, err := parseBackup(rawBackup)
	if err != nil {
		slog.Warn("discarding invalid DNS backup", "err", err)
		if clearErr := c.removeKey(backupKey); clearErr != nil {
			return errors.Join(err, fmt.Errorf("remove DNS backup: %w", clearErr))
		}
		return err
	}
	if hadSetupInitially {
		if err := c.restoreDNS(service); err != nil {
			return fmt.Errorf("restore DNS backup for %s: %w", service, err)
		}
	} else {
		if err := c.removeKey(setupKey(service)); err != nil {
			return fmt.Errorf("remove VPN DNS for %s: %w", service, err)
		}
	}

	c.flushCache()
	if err := c.removeKey(backupKey); err != nil {
		slog.Error("failed to remove restored DNS backup", "err", err)
	}
	return nil
}

func (c *Configurator) applyResolvers(service string, resolvers []string) error {
	script := "d.init\n" +
		"d.add ServerAddresses * " + strings.Join(resolvers, " ") + "\n" +
		"set " + setupKey(service) + "\n"
	return c.runScript(script)
}

func parseBackup(dictionary string) (string, bool, error) {
	service, ok := property(dictionary, serviceProperty)
	if !ok || service == "" || strings.ContainsAny(service, "/\r\n") {
		return "", false, errors.New("DNS backup is invalid: service is missing or invalid")
	}
	value, ok := property(dictionary, hadSetupInitiallyProperty)
	if !ok {
		return "", false, errors.New("DNS backup is invalid: initial setup marker is missing")
	}
	hadSetupInitially, err := strconv.ParseBool(value)
	if err != nil {
		return "", false, fmt.Errorf("DNS backup is invalid: parse initial setup marker %q: %w", value, err)
	}
	return service, hadSetupInitially, nil
}

func (c *Configurator) removeKey(key string) error {
	output, err := c.runner.CombinedOutputWithInput("scutil", strings.NewReader("remove "+key+"\n"))
	if err != nil {
		return commandError("scutil", output, err)
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" || detail == "No such key" {
		return nil
	}
	return fmt.Errorf("scutil: %s", detail)
}

func (c *Configurator) primaryService() (string, error) {
	for _, key := range []string{globalIPv4Key, globalIPv6Key} {
		state, exists, err := c.show(key)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		if service, ok := property(state, "PrimaryService"); ok && service != "" {
			return service, nil
		}
	}
	return "", errors.New("find primary network service: no IPv4 or IPv6 primary service")
}

func (c *Configurator) show(key string) (string, bool, error) {
	output, err := c.runner.CombinedOutputWithInput("scutil", strings.NewReader("show "+key+"\n"))
	if err != nil {
		return "", false, commandError("scutil", output, err)
	}
	text := strings.TrimSpace(string(output))
	if text == "No such key" {
		return "", false, nil
	}
	if strings.HasPrefix(text, "<dictionary> {") && strings.HasSuffix(text, "}") {
		return text, true, nil
	}
	return "", false, fmt.Errorf("scutil: unexpected output: %q", text)
}

func (c *Configurator) runScript(script string) error {
	output, err := c.runner.CombinedOutputWithInput("scutil", strings.NewReader(script))
	if err != nil {
		return commandError("scutil", output, err)
	}
	detail := strings.TrimSpace(string(output))
	if detail != "" {
		return fmt.Errorf("scutil: %s", detail)
	}
	return nil
}

func (c *Configurator) flushCache() {
	_, _ = c.runner.CombinedOutput("dscacheutil", "-flushcache")
}

func setupKey(service string) string {
	return "Setup:/Network/Service/" + service + "/DNS"
}

func (c *Configurator) backupDNS(service string, hadSetupInitially bool) error {
	initialCommand := "d.init"
	if hadSetupInitially {
		initialCommand = "get " + setupKey(service)
	}
	script := initialCommand + "\n" +
		"d.add " + serviceProperty + " " + service + "\n" +
		"d.add " + hadSetupInitiallyProperty + " " + strconv.FormatBool(hadSetupInitially) + "\n" +
		"set " + backupKey + "\n"
	return c.runScript(script)
}

func (c *Configurator) restoreDNS(service string) error {
	script := "get " + backupKey + "\n" +
		"d.remove " + serviceProperty + "\n" +
		"d.remove " + hadSetupInitiallyProperty + "\n" +
		"set " + setupKey(service) + "\n"
	return c.runScript(script)
}

func property(dictionary, name string) (string, bool) {
	for line := range strings.SplitSeq(dictionary, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.TrimSpace(key) == name {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func commandError(name string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s: %w: %s", name, err, detail)
}
