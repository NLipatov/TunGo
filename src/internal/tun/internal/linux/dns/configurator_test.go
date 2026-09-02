package dns

import (
	"errors"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type recordedCommand struct {
	name  string
	args  []string
	input string
}

type runnerMock struct {
	calls    []recordedCommand
	failAt   int
	errorsAt map[int]error
}

func (m *runnerMock) CombinedOutput(name string, args ...string) ([]byte, error) {
	return m.record(name, args, "")
}

func (m *runnerMock) CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	return m.record(name, args, string(data))
}

func (m *runnerMock) record(name string, args []string, input string) ([]byte, error) {
	m.calls = append(m.calls, recordedCommand{name: name, args: append([]string(nil), args...), input: input})
	if err := m.errorsAt[len(m.calls)]; err != nil {
		return []byte("command failed"), err
	}
	if m.failAt == len(m.calls) {
		return []byte("command failed"), errors.New("exit status 1")
	}
	return nil, nil
}

func TestConfiguratorReportsUnavailableResolvedStubOwner(t *testing.T) {
	runner := &runnerMock{errorsAt: map[int]error{1: exec.ErrNotFound}}
	configurator := New(runner)

	selected, err := configurator.detectLinkBackend("/run/systemd/resolve/stub-resolv.conf")
	if selected != unknown || err == nil ||
		!strings.Contains(err.Error(), "/etc/resolv.conf points to systemd-resolved") ||
		!strings.Contains(err.Error(), "resolvectl") {
		t.Fatalf("detectLinkBackend() = %d, %v; want unavailable systemd-resolved diagnostic", selected, err)
	}

	want := []recordedCommand{{name: "resolvectl", args: []string{"status"}}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands = %#v, want only owner probe %#v", runner.calls, want)
	}
}

func TestConfiguratorIgnoresUnknownResolvConfOwner(t *testing.T) {
	runner := &runnerMock{}
	configurator := New(runner)

	selected, err := configurator.detectLinkBackend("/run/custom-resolver/resolv.conf")
	if selected != unknown || err != nil {
		t.Fatalf("detectLinkBackend() = %d, %v; want no backend", selected, err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("commands = %v, want no backend probes", runner.calls)
	}
}

func TestConfiguratorDetectsSystemdResolvedLinks(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
	}{
		{name: "stub", target: "../run/systemd/resolve/stub-resolv.conf"},
		{name: "uplink", target: "/run/systemd/resolve/resolv.conf"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &runnerMock{}
			configurator := New(runner)

			selected, err := configurator.detectLinkBackend(test.target)
			if selected != resolved || err != nil {
				t.Fatalf("detectLinkBackend() = %d, %v; want %d, nil", selected, err, resolved)
			}

			want := []recordedCommand{{name: "resolvectl", args: []string{"status"}}}
			if !reflect.DeepEqual(runner.calls, want) {
				t.Fatalf("commands = %#v, want %#v", runner.calls, want)
			}
		})
	}
}

func TestConfiguratorDetectsResolvconfFromResolvConf(t *testing.T) {
	runner := &runnerMock{}
	configurator := New(runner)

	selected, err := configurator.detectLinkBackend("../run/resolvconf/resolv.conf")
	if selected != resolvconf || err != nil {
		t.Fatalf("detectLinkBackend() = %d, %v; want %d, nil", selected, err, resolvconf)
	}

	want := []recordedCommand{{name: "resolvconf", args: []string{"-l"}}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands = %#v, want %#v", runner.calls, want)
	}
}

func TestUsesResolvedStub(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     bool
	}{
		{name: "stub", contents: "nameserver 127.0.0.53\n", want: true},
		{name: "whitespace", contents: "  nameserver   127.0.0.53  # local stub\n", want: true},
		{name: "comment", contents: "# nameserver 127.0.0.53\n"},
		{name: "different loopback", contents: "nameserver 127.0.0.1\n"},
		{name: "missing address", contents: "nameserver\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usesResolvedStub(test.contents); got != test.want {
				t.Fatalf("usesResolvedStub() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestConfiguratorDetectsResolvedStub(t *testing.T) {
	runner := &runnerMock{}
	configurator := New(runner)

	selected, err := configurator.detectStubBackend("nameserver 127.0.0.53\n")
	if selected != resolved || err != nil {
		t.Fatalf("detectStubBackend() = %d, %v; want %d, nil", selected, err, resolved)
	}
	want := []recordedCommand{{name: "resolvectl", args: []string{"status"}}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands = %#v, want %#v", runner.calls, want)
	}
}

func TestConfiguratorReportsUnavailableResolvedStub(t *testing.T) {
	runner := &runnerMock{errorsAt: map[int]error{1: exec.ErrNotFound}}
	configurator := New(runner)

	selected, err := configurator.detectStubBackend("nameserver 127.0.0.53\n")
	if selected != unknown || err == nil || !strings.Contains(err.Error(), "resolvectl is unavailable") {
		t.Fatalf("detectStubBackend() = %d, %v; want unavailable resolvectl diagnostic", selected, err)
	}
}

func TestConfiguratorResolvconfReusesOwnedKeyAfterCrash(t *testing.T) {
	runner := &runnerMock{}
	newConfigurator := func() *Configurator {
		return New(runner)
	}

	if err := newConfigurator().configure(resolvconf, "tun0", []string{"1.1.1.1"}, nil); err != nil {
		t.Fatalf("first configure() error = %v", err)
	}
	if err := newConfigurator().configure(resolvconf, "tun1", []string{"8.8.8.8"}, nil); err != nil {
		t.Fatalf("second configure() error = %v", err)
	}

	firstKey := runner.calls[0].args[1]
	secondKey := runner.calls[1].args[1]
	if firstKey != resolvconfKey || secondKey != firstKey {
		t.Fatalf("resolvconf keys = %q, %q, want stable %q", firstKey, secondKey, resolvconfKey)
	}
}

func TestConfiguratorRevertRemovesStaleResolvconfEntry(t *testing.T) {
	runner := &runnerMock{}
	configurator := New(runner)

	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	want := []recordedCommand{{name: "resolvconf", args: []string{"-d", resolvconfKey, "-f"}}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands = %#v, want stale entry cleanup %#v", runner.calls, want)
	}
}

func TestConfiguratorRevertWithoutResolvconfIsNoOp(t *testing.T) {
	runner := &runnerMock{errorsAt: map[int]error{1: exec.ErrNotFound}}
	configurator := New(runner)

	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v, want unavailable resolvconf ignored", err)
	}
}

func TestConfiguratorRevertReportsStaleResolvconfCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("cleanup failed")
	runner := &runnerMock{errorsAt: map[int]error{1: cleanupErr}}

	if err := New(runner).Revert(); !errors.Is(err, cleanupErr) {
		t.Fatalf("Revert() error = %v, want %v", err, cleanupErr)
	}
}

func TestConfiguratorRollsBackFailedResolvedSetup(t *testing.T) {
	runner := &runnerMock{failAt: 2}
	configurator := New(runner)

	err := configurator.configure(resolved, "tun0", []string{"1.1.1.1"}, nil)
	if err == nil || !strings.Contains(err.Error(), "make tun0 the default DNS route") {
		t.Fatalf("configure() error = %v", err)
	}
	if got := runner.calls[len(runner.calls)-1]; got.name != "resolvectl" || !reflect.DeepEqual(got.args, []string{"revert", "tun0"}) {
		t.Fatalf("rollback command = %#v", got)
	}
	if configurator.activeInterface != "" {
		t.Fatalf("activeInterface = %q after successful rollback", configurator.activeInterface)
	}
}

func TestConfiguratorRetainsFailedRevertForRetry(t *testing.T) {
	runner := &runnerMock{failAt: 4}
	configurator := New(runner)

	if err := configurator.configure(resolved, "tun0", []string{"1.1.1.1"}, nil); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if err := configurator.Revert(); err == nil {
		t.Fatal("Revert() error = nil")
	}
	if configurator.activeInterface != "tun0" {
		t.Fatalf("activeInterface = %q, want retained tun0", configurator.activeInterface)
	}

	runner.failAt = 0
	if err := configurator.Revert(); err != nil {
		t.Fatalf("retry Revert() error = %v", err)
	}
	if configurator.activeInterface != "" {
		t.Fatalf("activeInterface = %q after retry", configurator.activeInterface)
	}
}

func TestConfiguratorReconfiguresAfterFailedRevert(t *testing.T) {
	runner := &runnerMock{failAt: 4}
	configurator := New(runner)

	if err := configurator.configure(resolved, "tun0", []string{"1.1.1.1"}, nil); err != nil {
		t.Fatalf("first configure() error = %v", err)
	}
	if err := configurator.Revert(); err == nil {
		t.Fatal("Revert() error = nil")
	}
	if err := configurator.configure(resolved, "tun0", []string{"8.8.8.8"}, nil); err != nil {
		t.Fatalf("second configure() error = %v", err)
	}

	wantTail := []recordedCommand{
		{name: "resolvectl", args: []string{"domain", "tun0", "~."}},
		{name: "resolvectl", args: []string{"default-route", "tun0", "true"}},
		{name: "resolvectl", args: []string{"dns", "tun0", "8.8.8.8"}},
	}
	gotTail := runner.calls[len(runner.calls)-len(wantTail):]
	if !reflect.DeepEqual(gotTail, wantTail) {
		t.Fatalf("second configure() commands = %#v, want %#v", gotTail, wantTail)
	}
	if configurator.activeInterface != "tun0" || configurator.activeBackend != resolved {
		t.Fatalf(
			"active DNS = interface %q backend %d, want tun0/%d",
			configurator.activeInterface,
			configurator.activeBackend,
			resolved,
		)
	}
}

func TestConfiguratorMarksFailedRollbackForRetry(t *testing.T) {
	setupErr := errors.New("setup failed")
	cleanupErr := errors.New("cleanup failed")
	runner := &runnerMock{errorsAt: map[int]error{2: setupErr, 3: cleanupErr}}
	configurator := New(runner)

	err := configurator.configure(resolved, "tun0", []string{"1.1.1.1"}, nil)
	if !errors.Is(err, setupErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("configure() error = %v, want setup and cleanup causes", err)
	}
	if configurator.activeInterface != "tun0" {
		t.Fatalf("activeInterface = %q, want retained tun0", configurator.activeInterface)
	}
}

func TestConfiguratorConfiguresResolved(t *testing.T) {
	runner := &runnerMock{}
	configurator := New(runner)
	if err := configurator.configure(resolved, "tun0", []string{"1.1.1.1"}, nil); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	want := []recordedCommand{
		{name: "resolvectl", args: []string{"domain", "tun0", "~."}},
		{name: "resolvectl", args: []string{"default-route", "tun0", "true"}},
		{name: "resolvectl", args: []string{"dns", "tun0", "1.1.1.1"}},
		{name: "resolvectl", args: []string{"revert", "tun0"}},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands = %#v, want %#v", runner.calls, want)
	}
}

func TestConfiguratorConfiguresResolvconf(t *testing.T) {
	runner := &runnerMock{}
	configurator := New(runner)

	if err := configurator.configure(resolvconf, "tun0", []string{"1.1.1.1"}, nil); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	want := []recordedCommand{
		{name: "resolvconf", args: []string{"-a", resolvconfKey, "-m", "0", "-x"}, input: "nameserver 1.1.1.1\n"},
		{name: "resolvconf", args: []string{"-d", resolvconfKey, "-f"}},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands = %#v, want %#v", runner.calls, want)
	}
}
