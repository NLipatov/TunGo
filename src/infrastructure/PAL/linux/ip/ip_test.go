package ip

import (
	"errors"
	"strings"
	"testing"
)

type mockCommander struct {
	OutputFunc         func(name string, args ...string) ([]byte, error)
	CombinedOutputFunc func(name string, args ...string) ([]byte, error)
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
