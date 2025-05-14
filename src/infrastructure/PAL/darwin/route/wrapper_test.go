package route

import (
	"errors"
	"strings"
	"testing"
)

// fakeCommander implements PAL.Commander for tests.
type wrapperTestCommander struct {
	out map[string][]byte
	err map[string]error
	run map[string]error
}

func (f *wrapperTestCommander) Output(name string, _ ...string) ([]byte, error) {
	return f.out[name], f.err[name]
}

func (f *wrapperTestCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	key := name + "|" + strings.Join(args, " ")
	return f.out[key], f.err[key]
}

func (f *wrapperTestCommander) Run(name string, args ...string) error {
	key := name + "|" + strings.Join(args, " ")
	return f.run[key]
}

func newWrapper(out map[string][]byte, err map[string]error, run map[string]error) *Wrapper {
	return &Wrapper{commander: &wrapperTestCommander{out: out, err: err, run: run}}
}

func TestAdd(t *testing.T) {
	key := "route|add|1.2.3.4|-interface|eth0"

	// success
	w := newWrapper(map[string][]byte{key: nil}, nil, nil)
	if err := w.Add("1.2.3.4", "eth0"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	w = newWrapper(nil, map[string]error{key: errors.New("boom")}, nil)
	if err := w.Add("1.2.3.4", "eth0"); err == nil || !strings.Contains(err.Error(), "route add 1.2.3.4 via interface eth0 failed") {
		t.Errorf("unexpected Add error: %v", err)
	}
}

func TestAddViaGateway(t *testing.T) {
	key := "route|add|5.6.7.8|9.9.9.9"

	// success
	w := newWrapper(map[string][]byte{key: nil}, nil, nil)
	if err := w.AddViaGateway("5.6.7.8", "9.9.9.9"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// failure
	w = newWrapper(nil, map[string]error{key: errors.New("fail")}, nil)
	if err := w.AddViaGateway("5.6.7.8", "9.9.9.9"); err == nil || !strings.Contains(err.Error(), "route add 5.6.7.8 via 9.9.9.9 failed") {
		t.Errorf("unexpected AddViaGateway error: %v", err)
	}
}

func TestAddSplit(t *testing.T) {
	successOut := map[string][]byte{
		"route|-q|add|-net|0.0.0.0/1|-interface|tun0":   nil,
		"route|-q|add|-net|128.0.0.0/1|-interface|tun0": nil,
	}

	// success
	w := newWrapper(successOut, nil, nil)
	if err := w.AddSplit("tun0"); err != nil {
		t.Fatalf("expected AddSplit to succeed, got %v", err)
	}

	// fail first
	w = newWrapper(successOut, map[string]error{"route|-q|add|-net|0.0.0.0/1|-interface|tun0": errors.New("f1")}, nil)
	if err := w.AddSplit("tun0"); err == nil || !strings.Contains(err.Error(), "route add 0.0.0.0/1 failed") {
		t.Errorf("unexpected AddSplit error: %v", err)
	}

	// fail second
	w = newWrapper(successOut, map[string]error{"route|-q|add|-net|128.0.0.0/1|-interface|tun0": errors.New("f2")}, nil)
	if err := w.AddSplit("tun0"); err == nil || !strings.Contains(err.Error(), "route add 128.0.0.0/1 failed") {
		t.Errorf("unexpected AddSplit error: %v", err)
	}
}

func TestDelSplit(t *testing.T) {
	// both succeed
	runOK := map[string]error{
		"route|-q|delete|-net|0.0.0.0/1|-interface|tun0":   nil,
		"route|-q|delete|-net|128.0.0.0/1|-interface|tun0": nil,
	}
	w := newWrapper(nil, nil, runOK)
	if err := w.DelSplit("tun0"); err != nil {
		t.Fatalf("expected DelSplit to succeed, got %v", err)
	}

	// one fails
	runFail := map[string]error{"route|-q|delete|-net|0.0.0.0/1|-interface|tun0": errors.New("boom")}
	w = newWrapper(nil, nil, runFail)
	if err := w.DelSplit("tun0"); err == nil {
		t.Errorf("expected DelSplit to fail, got nil")
	}
}

func TestDel(t *testing.T) {
	key := "route|delete|9.9.9.9"

	// success
	w := newWrapper(map[string][]byte{key: nil}, nil, nil)
	if err := w.Del("9.9.9.9"); err != nil {
		t.Fatalf("expected Del to succeed, got %v", err)
	}

	// failure
	w = newWrapper(nil, map[string]error{key: errors.New("err")}, nil)
	if err := w.Del("9.9.9.9"); err == nil || !strings.Contains(err.Error(), "route delete 9.9.9.9 failed") {
		t.Errorf("unexpected Del error: %v", err)
	}
}

func TestDefaultGateway(t *testing.T) {
	key := "route|-n|get|default"

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
	w := newWrapper(nil, map[string]error{"route|-n|get|10.0.0.1": errors.New("x")}, nil)
	if err := w.Get("10.0.0.1"); err == nil || !strings.Contains(err.Error(), "route get 10.0.0.1") {
		t.Errorf("expected Get parseRoute error, got %v", err)
	}

	// 2) gateway non-empty
	out2 := map[string][]byte{
		"route|-n|get|1.1.1.1":              []byte("gateway: 8.8.8.8\ninterface: eth0\n"),
		"route|add|8.8.8.8|-interface|eth0": nil,
		"route|add|1.1.1.1|8.8.8.8":         nil,
	}
	w = newWrapper(out2, nil, nil)
	if err := w.Get("1.1.1.1"); err != nil {
		t.Errorf("expected Get to succeed, got %v", err)
	}

	// 3) loopback gateway => fallback to default
	out3 := map[string][]byte{
		"route|-n|get|127.0.0.1":          []byte("gateway: 127.0.0.1\ninterface: e0\n"),
		"route|-n|get|default":            []byte("gateway: 2.2.2.2\ninterface: e1\n"),
		"route|add|2.2.2.2|-interface|e1": nil,
		"route|add|9.9.9.9|2.2.2.2":       nil,
	}
	w = newWrapper(out3, nil, nil)
	if err := w.Get("127.0.0.1"); err != nil {
		t.Errorf("expected fallback Get to succeed, got %v", err)
	}

	// 4) iface-only
	out4 := map[string][]byte{"route|-n|get|5.5.5.5": []byte("interface: e2\n")}
	w = newWrapper(out4, nil, nil)
	if err := w.Get("5.5.5.5"); err != nil {
		t.Errorf("expected iface-only Get to succeed, got %v", err)
	}

	// 5) neither gw nor iface
	out5 := map[string][]byte{"route|-n|get|9.9.9.9": []byte("foo")}
	w = newWrapper(out5, nil, nil)
	if err := w.Get("9.9.9.9"); err == nil || !strings.Contains(err.Error(), "no route found") {
		t.Errorf("expected no-route error, got %v", err)
	}

	// 6) Add(...) fails
	out6 := map[string][]byte{
		"route|-n|get|3.3.3.3":            []byte("gateway: 3.3.3.1\ninterface: e3\n"),
		"route|add|3.3.3.1|-interface|e3": nil,
	}
	w = newWrapper(out6, map[string]error{"route|add|3.3.3.1|-interface|e3": errors.New("boom")}, nil)
	if err := w.Get("3.3.3.3"); err == nil || !strings.Contains(err.Error(), "route keep gw 3.3.3.1") {
		t.Errorf("expected keep-gw error, got %v", err)
	}
}
