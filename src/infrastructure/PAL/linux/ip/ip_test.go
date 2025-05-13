package ip

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type mockCommander struct {
	commands []string
	Stdout   []byte
	Stderr   []byte
	Err      error
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.commands = append(m.commands, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
	return m.Stderr, m.Err
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	m.commands = append(m.commands, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
	return m.Stdout, m.Err
}

func TestTunTapAddDevTun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockCommander{}
		w := NewWrapper(mock)

		err := w.TunTapAddDevTun("tun0")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("error", func(t *testing.T) {
		mock := &mockCommander{Stderr: []byte("fail"), Err: errors.New("exec error")}
		w := NewWrapper(mock)

		err := w.TunTapAddDevTun("tun0")
		if err == nil || !strings.Contains(err.Error(), "failed to create TUN") {
			t.Errorf("expected error, got: %v", err)
		}
	})
}

func TestLinkDelete(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.LinkDelete("tun0")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLinkSetDevUp(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.LinkSetDevUp("tun0")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddrAddDev(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.AddrAddDev("tun0", "10.0.0.1/24")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouteDefault(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockCommander{Stdout: []byte("default via 10.0.0.1 dev eth0")}
		w := NewWrapper(mock)

		iface, err := w.RouteDefault()
		if err != nil || iface != "eth0" {
			t.Errorf("unexpected: iface=%s, err=%v", iface, err)
		}
	})

	t.Run("no default route", func(t *testing.T) {
		mock := &mockCommander{Stdout: []byte("no default route")}
		w := NewWrapper(mock)

		_, err := w.RouteDefault()
		if err == nil {
			t.Errorf("expected error")
		}
	})
}

func TestRouteAddDefaultDev(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.RouteAddDefaultDev("tun0")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouteGet(t *testing.T) {
	mock := &mockCommander{Stdout: []byte("10.0.0.1 via dev eth0")}
	w := NewWrapper(mock)

	route, err := w.RouteGet("1.1.1.1")
	if err != nil || route != "10.0.0.1 via dev eth0" {
		t.Errorf("unexpected route or error: %v %v", route, err)
	}
}

func TestRouteAddDev(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.RouteAddDev("1.1.1.1", "tun0")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouteAddViaDev(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.RouteAddViaDev("1.1.1.1", "tun0", "10.0.0.1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouteDel(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.RouteDel("1.1.1.1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLinkSetDevMTU(t *testing.T) {
	mock := &mockCommander{}
	w := NewWrapper(mock)

	err := w.LinkSetDevMTU("tun0", 1400)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddrShowDev(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockCommander{Stderr: []byte("10.0.0.1\n")}
		w := NewWrapper(mock)

		ipStr, err := w.AddrShowDev(4, "tun0")
		if err != nil || ipStr != "10.0.0.1" {
			t.Errorf("unexpected result: %v, %v", ipStr, err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		mock := &mockCommander{Stderr: []byte("\n")}
		w := NewWrapper(mock)

		_, err := w.AddrShowDev(4, "tun0")
		if err == nil {
			t.Errorf("expected error for empty output")
		}
	})
}
