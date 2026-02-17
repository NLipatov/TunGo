package ip

import (
	"errors"
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

func newWrapper(success bool, output string, err error) Contract {
	return NewWrapper(&mockCommander{
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
	failOnCall    int
}

func (m *recordingCommander) Run(_ string, _ ...string) error { return nil }
func (m *recordingCommander) Output(_ string, _ ...string) ([]byte, error) {
	return nil, nil
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
		err := newWrapper(true, "", nil).TunTapAddDevTun("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "error", errors.New("fail")).TunTapAddDevTun("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to create TUN") {
			t.Fatal("expected failure")
		}
	})
}

func TestLinkDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).LinkDelete("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "error", errors.New("fail")).LinkDelete("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to delete interface") {
			t.Fatal("expected failure")
		}
	})
}

func TestLinkSetDevUp(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).LinkSetDevUp("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).LinkSetDevUp("tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAddrAddDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).AddrAddDev("tun0", "10.0.0.1/24")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).AddrAddDev("tun0", "10.0.0.1/24")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteDefault(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		w := newWrapper(true, "default via 10.0.0.1 dev eth0\n", nil)
		iface, err := w.RouteDefault()
		if err != nil || iface != "eth0" {
			t.Fatal("failed to parse default route")
		}
	})
	t.Run("no default", func(t *testing.T) {
		w := newWrapper(true, "link-local route only", nil)
		_, err := w.RouteDefault()
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("command error", func(t *testing.T) {
		w := newWrapper(false, "output", errors.New("fail"))
		_, err := w.RouteDefault()
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddDefaultDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).RouteAddDefaultDev("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).RouteAddDefaultDev("tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRoute6AddDefaultDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).Route6AddDefaultDev("tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).Route6AddDefaultDev("tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddSplitDefaultDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := &recordingCommander{}
		w := NewWrapper(rec)
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
		w := NewWrapper(rec)
		err := w.RouteAddSplitDefaultDev("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to add split route 128.0.0.0/1") {
			t.Fatalf("expected split-route error, got %v", err)
		}
	})
}

func TestRoute6AddSplitDefaultDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := &recordingCommander{}
		w := NewWrapper(rec)
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
		w := NewWrapper(rec)
		err := w.Route6AddSplitDefaultDev("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to add IPv6 split route 8000::/1") {
			t.Fatalf("expected IPv6 split-route error, got %v", err)
		}
	})
}

func TestRouteDelSplitDefault(t *testing.T) {
	rec := &recordingCommander{failOnCall: 1}
	w := NewWrapper(rec)

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
	w := NewWrapper(rec)

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
	t.Run("success", func(t *testing.T) {
		route, err := newWrapper(true, "10.0.0.1 dev eth0", nil).RouteGet("1.1.1.1")
		if err != nil || !strings.Contains(route, "10.0.0.1") {
			t.Fatal("unexpected result", err, route)
		}
	})
	t.Run("error", func(t *testing.T) {
		_, err := newWrapper(false, "output", errors.New("fail")).RouteGet("1.1.1.1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).RouteAddDev("1.1.1.1", "tun0")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).RouteAddDev("1.1.1.1", "tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteAddViaDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).RouteAddViaDev("1.1.1.1", "tun0", "10.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).RouteAddViaDev("1.1.1.1", "tun0", "10.0.0.1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteDel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).RouteDel("1.1.1.1")
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).RouteDel("1.1.1.1")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLinkSetDevMTU(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := newWrapper(true, "", nil).LinkSetDevMTU("tun0", 1400)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("error", func(t *testing.T) {
		err := newWrapper(false, "output", errors.New("fail")).LinkSetDevMTU("tun0", 1400)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAddrShowDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		w := newWrapper(false, "10.0.0.1\n", nil)
		ipStr, err := w.AddrShowDev(4, "tun0")
		if err != nil || ipStr != "10.0.0.1" {
			t.Fatal("unexpected ip:", ipStr, err)
		}
	})
	t.Run("empty ip", func(t *testing.T) {
		w := newWrapper(false, "\n", nil)
		_, err := w.AddrShowDev(4, "tun0")
		if err == nil {
			t.Fatal("expected error on empty IP")
		}
	})
	t.Run("command error", func(t *testing.T) {
		w := newWrapper(false, "error", errors.New("fail"))
		_, err := w.AddrShowDev(4, "tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
