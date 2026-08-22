package ip

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

type mockCommander struct {
	OutputFunc         func(name string, args ...string) ([]byte, error)
	CombinedOutputFunc func(name string, args ...string) ([]byte, error)
}

func (m *mockCommander) Run(_ string, _ ...string) error {
	panic("not implemented")
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	return m.OutputFunc(name, args...)
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	return m.CombinedOutputFunc(name, args...)
}

func newConfigurator(success bool, output string, err error) Contract {
	return New(&mockCommander{
		OutputFunc: func(name string, args ...string) ([]byte, error) {
			if success {
				return []byte(output), nil
			}
			return []byte(output), err
		},
		CombinedOutputFunc: func(name string, args ...string) ([]byte, error) {
			if success {
				return []byte(output), nil
			}
			return []byte(output), err
		},
	})
}

type recordingCommander struct {
	combinedCalls [][]string
	outputCalls   [][]string
	output        []byte
	failOnCall    int
}

func (m *recordingCommander) Run(_ string, _ ...string) error { return nil }
func (m *recordingCommander) Output(name string, args ...string) ([]byte, error) {
	m.outputCalls = append(m.outputCalls, append([]string{name}, args...))
	return m.output, nil
}
func (m *recordingCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.combinedCalls = append(m.combinedCalls, call)
	if m.failOnCall > 0 && len(m.combinedCalls) == m.failOnCall {
		return []byte("boom"), errors.New("boom")
	}
	return nil, nil
}

func TestTunTapAddDevTun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newConfigurator(true, "", nil).TunTapAddDevTun("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "error", errors.New("fail")).TunTapAddDevTun("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to create TUN") {
			t.Fatal("expected failure")
		}
	})
}

func TestLinkDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newConfigurator(true, "", nil).LinkDelete("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "error", errors.New("fail")).LinkDelete("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to delete interface") {
			t.Fatal("expected failure")
		}
	})
}

func TestLinkSetDevUp(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newConfigurator(true, "", nil).LinkSetDevUp("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).LinkSetDevUp("tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAddrAddDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newConfigurator(true, "", nil).AddrAddDev("tun0", "10.0.0.1/24")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).AddrAddDev("tun0", "10.0.0.1/24")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteDefault(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		w := newConfigurator(true, "default via 10.0.0.1 dev eth0\n", nil)
		iface, err := w.RouteDefault()
		if err != nil || iface != "eth0" {
			t.Fatal("failed to parse default route")
		}
	})
	t.Run("no default", func(t *testing.T) {
		w := newConfigurator(true, "link-local route only", nil)
		_, err := w.RouteDefault()
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("command error", func(t *testing.T) {
		w := newConfigurator(false, "output", errors.New("fail"))
		_, err := w.RouteDefault()
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddSplitDefaultDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := &recordingCommander{}
		w := New(rec)
		if err := w.RouteAddSplitDefaultDev("tun0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := [][]string{
			{"ip", "route", "add", "0.0.0.0/1", "dev", "tun0"},
			{"ip", "route", "add", "128.0.0.0/1", "dev", "tun0"},
		}
		if !reflect.DeepEqual(rec.combinedCalls, want) {
			t.Fatalf("unexpected calls: got %v, want %v", rec.combinedCalls, want)
		}
	})

	t.Run("error on second route", func(t *testing.T) {
		rec := &recordingCommander{failOnCall: 2}
		w := New(rec)
		err := w.RouteAddSplitDefaultDev("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to add split route 128.0.0.0/1") {
			t.Fatalf("expected split-route error, got %v", err)
		}
	})
}

func TestRoute6AddSplitDefaultDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := &recordingCommander{}
		w := New(rec)
		if err := w.Route6AddSplitDefaultDev("tun0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := [][]string{
			{"ip", "-6", "route", "add", "::/1", "dev", "tun0"},
			{"ip", "-6", "route", "add", "8000::/1", "dev", "tun0"},
		}
		if !reflect.DeepEqual(rec.combinedCalls, want) {
			t.Fatalf("unexpected calls: got %v, want %v", rec.combinedCalls, want)
		}
	})

	t.Run("error on second route", func(t *testing.T) {
		rec := &recordingCommander{failOnCall: 2}
		w := New(rec)
		err := w.Route6AddSplitDefaultDev("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to add IPv6 split route 8000::/1") {
			t.Fatalf("expected IPv6 split-route error, got %v", err)
		}
	})
}

func TestRouteDelSplitDefault(t *testing.T) {
	rec := &recordingCommander{failOnCall: 1}
	w := New(rec)

	if err := w.RouteDelSplitDefault("tun0"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := [][]string{
		{"ip", "route", "del", "0.0.0.0/1", "dev", "tun0"},
		{"ip", "route", "del", "128.0.0.0/1", "dev", "tun0"},
	}
	if !reflect.DeepEqual(rec.combinedCalls, want) {
		t.Fatalf("unexpected calls: got %v, want %v", rec.combinedCalls, want)
	}
}

func TestRoute6DelSplitDefault(t *testing.T) {
	rec := &recordingCommander{failOnCall: 1}
	w := New(rec)

	if err := w.Route6DelSplitDefault("tun0"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := [][]string{
		{"ip", "-6", "route", "del", "::/1", "dev", "tun0"},
		{"ip", "-6", "route", "del", "8000::/1", "dev", "tun0"},
	}
	if !reflect.DeepEqual(rec.combinedCalls, want) {
		t.Fatalf("unexpected calls: got %v, want %v", rec.combinedCalls, want)
	}
}

func TestRouteGet(t *testing.T) {
	for _, test := range []struct {
		name string
		host string
		want []string
	}{
		{name: "IPv4", host: "1.1.1.1", want: []string{"ip", "-4", "route", "get", "1.1.1.1"}},
		{name: "IPv6", host: "2001:db8::1", want: []string{"ip", "-6", "route", "get", "2001:db8::1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingCommander{output: []byte("route dev eth0")}
			route, err := New(recorder).RouteGet(test.host)
			if err != nil || route != "route dev eth0" {
				t.Fatalf("RouteGet() = %q, %v", route, err)
			}
			if len(recorder.outputCalls) != 1 || !reflect.DeepEqual(recorder.outputCalls[0], test.want) {
				t.Fatalf("calls = %v, want %v", recorder.outputCalls, test.want)
			}
		})
	}
	t.Run("error", func(t *testing.T) {
		_, err := newConfigurator(false, "output", errors.New("fail")).RouteGet("1.1.1.1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddDev(t *testing.T) {
	recorder := &recordingCommander{}
	if err := New(recorder).RouteAddDev("2001:db8::1", "tun0"); err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-6", "route", "add", "2001:db8::1", "dev", "tun0"}
	if len(recorder.combinedCalls) != 1 || !reflect.DeepEqual(recorder.combinedCalls[0], want) {
		t.Fatalf("calls = %v, want %v", recorder.combinedCalls, want)
	}
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).RouteAddDev("1.1.1.1", "tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddViaDev(t *testing.T) {
	recorder := &recordingCommander{}
	if err := New(recorder).RouteAddViaDev("1.1.1.1", "tun0", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-4", "route", "add", "1.1.1.1", "via", "10.0.0.1", "dev", "tun0"}
	if len(recorder.combinedCalls) != 1 || !reflect.DeepEqual(recorder.combinedCalls[0], want) {
		t.Fatalf("calls = %v, want %v", recorder.combinedCalls, want)
	}
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).RouteAddViaDev("1.1.1.1", "tun0", "10.0.0.1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteDel(t *testing.T) {
	recorder := &recordingCommander{}
	if err := New(recorder).RouteDel("2001:db8::1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-6", "route", "del", "2001:db8::1"}
	if len(recorder.combinedCalls) != 1 || !reflect.DeepEqual(recorder.combinedCalls[0], want) {
		t.Fatalf("calls = %v, want %v", recorder.combinedCalls, want)
	}
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).RouteDel("1.1.1.1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestHostRouteArgsRejectsInvalidAddress(t *testing.T) {
	if _, err := hostRouteArgs("invalid", "route"); err == nil {
		t.Fatal("hostRouteArgs() error = nil")
	}
	if args, err := hostRouteArgs(netip.MustParseAddr("192.0.2.1").String(), "route"); err != nil || args[0] != "-4" {
		t.Fatalf("hostRouteArgs() = %v, %v", args, err)
	}
}

func TestLinkSetDevMTU(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newConfigurator(true, "", nil).LinkSetDevMTU("tun0", 1400)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).LinkSetDevMTU("tun0", 1400)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
