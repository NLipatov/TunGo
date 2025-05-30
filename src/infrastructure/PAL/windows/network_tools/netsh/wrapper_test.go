package netsh

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// fakeCommander records the last invocation and returns preset out/err.
type wrapperTestCommander struct {
	out map[string][]byte
	err map[string]error
}

func (f *wrapperTestCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	key := makeKey(name, args...)
	return f.out[key], f.err[key]
}

// These two methods are unused by netsh.Wrapper but must exist to satisfy PAL.Commander.
func (f *wrapperTestCommander) Output(_ string, _ ...string) ([]byte, error) { return nil, nil }
func (f *wrapperTestCommander) Run(_ string, _ ...string) error              { return nil }

// makeKey joins the command and its args into a unique lookup key.
func makeKey(name string, args ...string) string {
	return name + "|" + strings.Join(args, "|")
}

func newWrapperBehavior(out map[string][]byte, err map[string]error) *Wrapper {
	return &Wrapper{commander: &wrapperTestCommander{out: out, err: err}}
}

func TestRouteDelete(t *testing.T) {
	host := "10.0.0.1"
	cmd := "route"
	key := makeKey(cmd, "delete", host)

	// success
	w := newWrapperBehavior(map[string][]byte{key: nil}, nil)
	if err := w.RouteDelete(host); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	e := errors.New("boom")
	w = newWrapperBehavior(nil, map[string]error{key: e})
	err := w.RouteDelete(host)
	if err == nil || !strings.Contains(err.Error(), "RouteDelete error: boom") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInterfaceIPSetAddressStatic(t *testing.T) {
	ifName, ipAddr, mask, gw := "eth0", "1.2.3.4", "255.255.255.0", "1.2.3.1"
	cmd := "netsh"
	args := []string{
		"interface", "ip", "set", "address",
		"name=" + ifName, "static", ipAddr, mask, gw, "1",
	}
	key := makeKey(cmd, args...)

	// success
	w := newWrapperBehavior(map[string][]byte{key: nil}, nil)
	if err := w.InterfaceIPSetAddressStatic(ifName, ipAddr, mask, gw); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	e := errors.New("fail")
	w = newWrapperBehavior(nil, map[string]error{key: e})
	err := w.InterfaceIPSetAddressStatic(ifName, ipAddr, mask, gw)
	if err == nil || !strings.Contains(err.Error(), "InterfaceIPSetAddressStatic error: fail") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInterfaceIPV4AddRouteDefault(t *testing.T) {
	ifName, gw := "eth1", "1.1.1.1"
	cmd := "netsh"
	args := []string{
		"interface", "ipv4", "add", "route", "0.0.0.0/0",
		"name=" + ifName, gw, "metric=1",
	}
	key := makeKey(cmd, args...)

	// success
	w := newWrapperBehavior(map[string][]byte{key: nil}, nil)
	if err := w.InterfaceIPV4AddRouteDefault(ifName, gw); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	e := errors.New("oops")
	w = newWrapperBehavior(nil, map[string]error{key: e})
	err := w.InterfaceIPV4AddRouteDefault(ifName, gw)
	if err == nil || !strings.Contains(err.Error(), "InterfaceIPV4AddRouteDefault error: oops") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInterfaceIPV4DeleteAddress(t *testing.T) {
	ifName := "eth2"
	cmd := "netsh"
	args := []string{
		"interface", "ipv4", "delete", "route", "0.0.0.0/0",
		"name=" + ifName,
	}
	key := makeKey(cmd, args...)

	// success
	w := newWrapperBehavior(map[string][]byte{key: nil}, nil)
	if err := w.InterfaceIPV4DeleteAddress(ifName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	e := errors.New("del-fail")
	w = newWrapperBehavior(nil, map[string]error{key: e})
	err := w.InterfaceIPV4DeleteAddress(ifName)
	if err == nil || !strings.Contains(err.Error(), "InterfaceIPV4DeleteAddress error: del-fail") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInterfaceIPDeleteAddress(t *testing.T) {
	ifName, addr := "eth3", "5.6.7.8"
	cmd := "netsh"
	args := []string{"interface", "ip", "delete", "address", "name=" + ifName, "addr=" + addr}
	key := makeKey(cmd, args...)

	// success
	w := newWrapperBehavior(map[string][]byte{key: nil}, nil)
	if err := w.InterfaceIPDeleteAddress(ifName, addr); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	e := errors.New("addr-fail")
	w = newWrapperBehavior(nil, map[string]error{key: e})
	err := w.InterfaceIPDeleteAddress(ifName, addr)
	if err == nil || !strings.Contains(err.Error(), "InterfaceIPDeleteAddress error: addr-fail") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetInterfaceMetric(t *testing.T) {
	ifName, metric := "eth4", 42
	cmd := "netsh"
	args := []string{"interface", "ipv4", "set", "interface", ifName, "metric=" + strconv.Itoa(metric)}
	key := makeKey(cmd, args...)

	// success
	w := newWrapperBehavior(map[string][]byte{key: nil}, nil)
	if err := w.SetInterfaceMetric(ifName, metric); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	e := errors.New("met-fail")
	w = newWrapperBehavior(nil, map[string]error{key: e})
	err := w.SetInterfaceMetric(ifName, metric)
	if err == nil || !strings.Contains(err.Error(), "SetInterfaceMetric error: met-fail") {
		t.Errorf("unexpected error: %v", err)
	}
}
