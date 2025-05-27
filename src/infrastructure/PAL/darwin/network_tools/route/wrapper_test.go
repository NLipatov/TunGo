package route

import (
	"errors"
	"strings"
	"testing"
)

// wrapperTestCommander implements PAL.Commander for tests.
type wrapperTestCommander struct {
	out map[string][]byte
	err map[string]error
	run map[string]error
}

func (f *wrapperTestCommander) Output(_ string, _ ...string) ([]byte, error) {
	// Not used by this wrapper
	return nil, nil
}

func (f *wrapperTestCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	key := makeKey(name, args...)
	return f.out[key], f.err[key]
}

func (f *wrapperTestCommander) Run(name string, args ...string) error {
	key := makeKey(name, args...)
	return f.run[key]
}

func makeKey(name string, args ...string) string {
	return name + "|" + strings.Join(args, " ")
}

func newWrapper(out map[string][]byte, cmdErr map[string]error, runErr map[string]error) *Wrapper {
	return &Wrapper{commander: &wrapperTestCommander{out: out, err: cmdErr, run: runErr}}
}

func TestAdd(t *testing.T) {
	key := makeKey("route", "add", "1.2.3.4", "-interface", "eth0")

	// success
	w := newWrapper(map[string][]byte{key: nil}, nil, nil)
	if err := w.Add("1.2.3.4", "eth0"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	w = newWrapper(nil, map[string]error{key: errors.New("boom")}, nil)
	err := w.Add("1.2.3.4", "eth0")
	if err == nil || !strings.Contains(err.Error(), "route add 1.2.3.4 via interface eth0 failed") {
		t.Errorf("unexpected Add error: %v", err)
	}
}

func TestAddViaGateway(t *testing.T) {
	key := makeKey("route", "add", "5.6.7.8", "9.9.9.9")

	// success
	w := newWrapper(map[string][]byte{key: nil}, nil, nil)
	if err := w.AddViaGateway("5.6.7.8", "9.9.9.9"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	w = newWrapper(nil, map[string]error{key: errors.New("fail")}, nil)
	err := w.AddViaGateway("5.6.7.8", "9.9.9.9")
	if err == nil || !strings.Contains(err.Error(), "route add 5.6.7.8 via 9.9.9.9 failed") {
		t.Errorf("unexpected AddViaGateway error: %v", err)
	}
}

func TestAddSplit(t *testing.T) {
	k1 := makeKey("route", "-q", "add", "-net", "0.0.0.0/1", "-interface", "tun0")
	k2 := makeKey("route", "-q", "add", "-net", "128.0.0.0/1", "-interface", "tun0")

	// success
	w := newWrapper(map[string][]byte{k1: nil, k2: nil}, nil, nil)
	if err := w.AddSplit("tun0"); err != nil {
		t.Fatalf("expected AddSplit to succeed, got %v", err)
	}

	// fail first
	w = newWrapper(nil, map[string]error{k1: errors.New("e1")}, nil)
	if err := w.AddSplit("tun0"); err == nil || !strings.Contains(err.Error(), "route add 0.0.0.0/1 failed") {
		t.Errorf("unexpected AddSplit error for first: %v", err)
	}

	// fail second
	w = newWrapper(map[string][]byte{k1: nil}, map[string]error{k2: errors.New("e2")}, nil)
	if err := w.AddSplit("tun0"); err == nil || !strings.Contains(err.Error(), "route add 128.0.0.0/1 failed") {
		t.Errorf("unexpected AddSplit error for second: %v", err)
	}
}

func TestDelSplit(t *testing.T) {
	k1 := makeKey("route", "-q", "delete", "-net", "0.0.0.0/1", "-interface", "tun0")
	k2 := makeKey("route", "-q", "delete", "-net", "128.0.0.0/1", "-interface", "tun0")

	// both succeed
	w := newWrapper(nil, nil, map[string]error{k1: nil, k2: nil})
	if err := w.DelSplit("tun0"); err != nil {
		t.Fatalf("expected DelSplit to succeed, got %v", err)
	}

	// one fails
	w = newWrapper(nil, nil, map[string]error{k1: errors.New("boom")})
	if err := w.DelSplit("tun0"); err == nil {
		t.Errorf("expected DelSplit to fail, got nil")
	}
}

func TestDel(t *testing.T) {
	key := makeKey("route", "delete", "9.9.9.9")

	// success
	w := newWrapper(map[string][]byte{key: nil}, nil, nil)
	if err := w.Del("9.9.9.9"); err != nil {
		t.Fatalf("expected Del to succeed, got %v", err)
	}

	// failure
	w = newWrapper(nil, map[string]error{key: errors.New("err")}, nil)
	err := w.Del("9.9.9.9")
	if err == nil || !strings.Contains(err.Error(), "route delete 9.9.9.9 failed") {
		t.Errorf("unexpected Del error: %v", err)
	}
}

func TestDefaultGateway(t *testing.T) {
	key := makeKey("route", "-n", "get", "default")

	// found
	out := []byte("gateway: 1.2.3.4\nfoo")
	w := newWrapper(map[string][]byte{key: out}, nil, nil)
	gw, err := w.DefaultGateway()
	if err != nil {
		t.Fatalf("expected DefaultGateway to succeed, got %v", err)
	}
	if gw != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", gw)
	}

	// not found
	w = newWrapper(map[string][]byte{key: []byte("nothing")}, nil, nil)
	if _, err := w.DefaultGateway(); err == nil || !strings.Contains(err.Error(), "no gateway found") {
		t.Errorf("unexpected DefaultGateway error: %v", err)
	}

	// command error
	w = newWrapper(nil, map[string]error{key: errors.New("fail")}, nil)
	if _, err := w.DefaultGateway(); err == nil || !strings.Contains(err.Error(), "defaultGateway") {
		t.Errorf("unexpected DefaultGateway error: %v", err)
	}
}

func TestGetAllBranches(t *testing.T) {
	// 1) parseRoute(destIP) fails
	kBad := makeKey("route", "-n", "get", "10.0.0.1")
	w := newWrapper(nil, map[string]error{kBad: errors.New("fail")}, nil)
	if err := w.Get("10.0.0.1"); err == nil || !strings.Contains(err.Error(), "route get 10.0.0.1") {
		t.Errorf("expected parseRoute error, got %v", err)
	}

	// 2) gateway non-empty
	kGet := makeKey("route", "-n", "get", "1.1.1.1")
	kAddIf := makeKey("route", "add", "8.8.8.8", "-interface", "eth0")
	kVia := makeKey("route", "add", "1.1.1.1", "8.8.8.8")
	w = newWrapper(map[string][]byte{
		kGet:   []byte("gateway: 8.8.8.8\ninterface: eth0\n"),
		kAddIf: nil,
		kVia:   nil,
	}, nil, nil)
	if err := w.Get("1.1.1.1"); err != nil {
		t.Errorf("expected Get to succeed, got %v", err)
	}

	// 3) loopback gateway → fallback to default
	kLB := makeKey("route", "-n", "get", "127.0.0.1")
	kDef := makeKey("route", "-n", "get", "default")
	kAddDefIf := makeKey("route", "add", "2.2.2.2", "-interface", "e1")
	kViaDef := makeKey("route", "add", "9.9.9.9", "2.2.2.2")
	w = newWrapper(map[string][]byte{
		kLB:       []byte("gateway: 127.0.0.1\ninterface: e0\n"),
		kDef:      []byte("gateway: 2.2.2.2\ninterface: e1\n"),
		kAddDefIf: nil,
		kViaDef:   nil,
	}, nil, nil)
	if err := w.Get("127.0.0.1"); err != nil {
		t.Errorf("expected fallback Get to succeed, got %v", err)
	}

	// 4) iface-only (must supply Add key too)
	kIO := makeKey("route", "-n", "get", "5.5.5.5")
	kAddIO := makeKey("route", "add", "5.5.5.5", "-interface", "e2")
	w = newWrapper(map[string][]byte{
		kIO:    []byte("interface: e2\n"),
		kAddIO: nil,
	}, nil, nil)
	if err := w.Get("5.5.5.5"); err != nil {
		t.Errorf("expected iface-only Get to succeed, got %v", err)
	}

	// 5) neither gw nor iface
	kNR := makeKey("route", "-n", "get", "9.9.9.9")
	w = newWrapper(map[string][]byte{kNR: []byte("foo")}, nil, nil)
	if err := w.Get("9.9.9.9"); err == nil || !strings.Contains(err.Error(), "no route found") {
		t.Errorf("expected no-route error, got %v", err)
	}

	// 6) Add(...) fails
	kG6 := makeKey("route", "-n", "get", "3.3.3.3")
	kAdd6 := makeKey("route", "add", "3.3.3.1", "-interface", "e3")
	w = newWrapper(map[string][]byte{
		kG6: []byte("gateway: 3.3.3.1\ninterface: e3\n"),
	}, map[string]error{kAdd6: errors.New("boom")}, nil)
	if err := w.Get("3.3.3.3"); err == nil || !strings.Contains(err.Error(), "route keep gw 3.3.3.1") {
		t.Errorf("expected keep-gw error, got %v", err)
	}
}
