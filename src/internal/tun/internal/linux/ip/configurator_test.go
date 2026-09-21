package ip

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

type mockRunner struct {
	OutputFunc                  func(name string, args ...string) ([]byte, error)
	CombinedOutputFunc          func(name string, args ...string) ([]byte, error)
	CombinedOutputWithInputFunc func(name string, input io.Reader, args ...string) ([]byte, error)
}

func (m *mockRunner) Run(_ string, _ ...string) error {
	panic("not implemented")
}

func (m *mockRunner) Output(name string, args ...string) ([]byte, error) {
	return m.OutputFunc(name, args...)
}

func (m *mockRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return m.CombinedOutputFunc(name, args...)
}

func (m *mockRunner) CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error) {
	return m.CombinedOutputWithInputFunc(name, input, args...)
}

func newConfigurator(success bool, output string, err error) Contract {
	return New(&mockRunner{
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

type recordingRunner struct {
	combinedCalls [][]string
	inputs        []string
	outputCalls   [][]string
	output        []byte
	failOnCall    int
}

func (m *recordingRunner) Run(_ string, _ ...string) error { return nil }
func (m *recordingRunner) Output(name string, args ...string) ([]byte, error) {
	m.outputCalls = append(m.outputCalls, append([]string{name}, args...))
	return m.output, nil
}
func (m *recordingRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.combinedCalls = append(m.combinedCalls, call)
	if m.failOnCall > 0 && len(m.combinedCalls) == m.failOnCall {
		return []byte("boom"), errors.New("boom")
	}
	return nil, nil
}

func (m *recordingRunner) CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	m.inputs = append(m.inputs, string(data))
	return m.CombinedOutput(name, args...)
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
	commandErr := errors.New("command failed")
	for _, test := range []struct {
		name    string
		output  string
		err     error
		wantErr bool
	}{
		{name: "success"},
		{name: "already absent", output: "Cannot find device \"tun0\"\n", err: commandErr},
		{name: "disappeared during deletion", output: "RTNETLINK answers: No such device\n", err: commandErr},
		{name: "permission denied", output: "RTNETLINK answers: Operation not permitted\n", err: commandErr, wantErr: true},
		{name: "device busy", output: "RTNETLINK answers: Device or resource busy\n", err: commandErr, wantErr: true},
		{name: "missing file", output: "RTNETLINK answers: No such file or directory\n", err: commandErr, wantErr: true},
		{name: "command could not start", err: commandErr, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := newConfigurator(test.err == nil, test.output, test.err).LinkDelete("tun0")
			if !test.wantErr {
				if err != nil {
					t.Fatalf("LinkDelete() error = %v", err)
				}
				return
			}
			if !errors.Is(err, test.err) {
				t.Fatalf("LinkDelete() error = %v, want wrapped %v", err, test.err)
			}
			if !strings.Contains(err.Error(), "tun0") || !strings.Contains(err.Error(), strings.TrimSpace(test.output)) {
				t.Fatalf("LinkDelete() error = %v, want interface name and command output", err)
			}
		})
	}
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

func TestSplitRoutesEmpty(t *testing.T) {
	for name, run := range map[string]func(*Configurator, string, []string) error{
		"add IPv4":    (*Configurator).RouteAddSplitDev,
		"delete IPv4": (*Configurator).RouteDelSplitDefault,
		"add IPv6":    (*Configurator).Route6AddSplitDev,
		"delete IPv6": (*Configurator).Route6DelSplitDefault,
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recordingRunner{}
			configurator := New(rec)
			for _, prefixes := range [][]string{nil, {}} {
				if err := run(configurator, "tun0", prefixes); err != nil {
					t.Fatalf("empty routes: %v", err)
				}
				if len(rec.combinedCalls) != 0 {
					t.Fatalf("empty routes ran commands: %v", rec.combinedCalls)
				}
			}
		})
	}
}

func TestRouteAddSplitDev(t *testing.T) {
	for _, tt := range []struct {
		name   string
		family string
		add    func(*Configurator, string, []string) error
		splits []string
		input  string
	}{
		{
			name: "IPv4", family: "-4", add: (*Configurator).RouteAddSplitDev,
			splits: []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"},
			input:  "route add 192.0.2.0/24 dev tun0\nroute add 198.51.100.0/24 dev tun0\nroute add 203.0.113.0/24 dev tun0\n",
		},
		{
			name: "IPv6", family: "-6", add: (*Configurator).Route6AddSplitDev,
			splits: []string{"2001:db8:1::/64", "2001:db8:2::/64", "2001:db8:3::/64"},
			input:  "route add 2001:db8:1::/64 dev tun0\nroute add 2001:db8:2::/64 dev tun0\nroute add 2001:db8:3::/64 dev tun0\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("single batch", func(t *testing.T) {
				rec := &recordingRunner{}
				if err := tt.add(New(rec), "tun0", tt.splits); err != nil {
					t.Fatal(err)
				}
				want := [][]string{{"ip", tt.family, "-batch", "-"}}
				if !reflect.DeepEqual(rec.combinedCalls, want) {
					t.Fatalf("calls = %v, want %v", rec.combinedCalls, want)
				}
				if !reflect.DeepEqual(rec.inputs, []string{tt.input}) {
					t.Fatalf("batch input = %q, want %q", rec.inputs, tt.input)
				}
			})
			t.Run("batch error", func(t *testing.T) {
				commandErr := errors.New("exit status 1")
				output := "RTNETLINK answers: File exists\nCommand failed -:2\n"
				runner := &mockRunner{
					CombinedOutputWithInputFunc: func(string, io.Reader, ...string) ([]byte, error) {
						return []byte(output), commandErr
					},
				}
				err := tt.add(New(runner), "tun0", tt.splits)
				if !errors.Is(err, commandErr) {
					t.Fatalf("error = %v, want wrapped %v", err, commandErr)
				}
				for _, text := range []string{"tun0", tt.family, output} {
					if !strings.Contains(err.Error(), text) {
						t.Fatalf("error = %v, want %q", err, text)
					}
				}
			})
		})
	}
}

func TestRouteAddSplitDevLargeBatch(t *testing.T) {
	splits := make([]string, 35000)
	for i := range splits {
		splits[i] = fmt.Sprintf("198.18.%d.%d/32", i/256, i%256)
	}
	rec := &recordingRunner{}
	if err := New(rec).RouteAddSplitDev("tun0", splits); err != nil {
		t.Fatal(err)
	}
	if len(rec.combinedCalls) != 1 || len(rec.inputs) != 1 {
		t.Fatalf("got %d calls and %d inputs, want one batch", len(rec.combinedCalls), len(rec.inputs))
	}
	lines := strings.Split(strings.TrimSuffix(rec.inputs[0], "\n"), "\n")
	if len(lines) != len(splits) {
		t.Fatalf("batch has %d lines, want %d", len(lines), len(splits))
	}
	for i, prefix := range splits {
		if want := "route add " + prefix + " dev tun0"; lines[i] != want {
			t.Fatalf("line %d = %q, want %q", i+1, lines[i], want)
		}
	}
}

func TestRouteDelSplitDefault(t *testing.T) {
	rec := &recordingRunner{failOnCall: 1}
	w := New(rec)

	if err := w.RouteDelSplitDefault("tun0", []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := [][]string{
		{"ip", "route", "del", "192.0.2.0/24", "dev", "tun0"},
		{"ip", "route", "del", "198.51.100.0/24", "dev", "tun0"},
		{"ip", "route", "del", "203.0.113.0/24", "dev", "tun0"},
	}
	if !reflect.DeepEqual(rec.combinedCalls, want) {
		t.Fatalf("unexpected calls: got %v, want %v", rec.combinedCalls, want)
	}
}

func TestRoute6DelSplitDefault(t *testing.T) {
	rec := &recordingRunner{failOnCall: 1}
	w := New(rec)

	if err := w.Route6DelSplitDefault("tun0", []string{"2001:db8:1::/64", "2001:db8:2::/64", "2001:db8:3::/64"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := [][]string{
		{"ip", "-6", "route", "del", "2001:db8:1::/64", "dev", "tun0"},
		{"ip", "-6", "route", "del", "2001:db8:2::/64", "dev", "tun0"},
		{"ip", "-6", "route", "del", "2001:db8:3::/64", "dev", "tun0"},
	}
	if !reflect.DeepEqual(rec.combinedCalls, want) {
		t.Fatalf("unexpected calls: got %v, want %v", rec.combinedCalls, want)
	}
}

func TestRouteGet(t *testing.T) {
	for _, test := range []struct {
		name string
		host netip.Addr
		want []string
	}{
		{name: "IPv4", host: netip.MustParseAddr("1.1.1.1"), want: []string{"ip", "-4", "route", "get", "1.1.1.1"}},
		{name: "IPv4-mapped IPv6", host: netip.MustParseAddr("::ffff:192.0.2.1"), want: []string{"ip", "-4", "route", "get", "192.0.2.1"}},
		{name: "IPv6", host: netip.MustParseAddr("2001:db8::1"), want: []string{"ip", "-6", "route", "get", "2001:db8::1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingRunner{output: []byte("route dev eth0")}
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
		_, err := newConfigurator(false, "output", errors.New("fail")).RouteGet(netip.MustParseAddr("1.1.1.1"))
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid address", func(t *testing.T) {
		if _, err := New(&recordingRunner{}).RouteGet(netip.Addr{}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteReplaceDev(t *testing.T) {
	for _, test := range []struct {
		name string
		host netip.Addr
		want []string
	}{
		{name: "IPv6", host: netip.MustParseAddr("2001:db8::1"), want: []string{"ip", "-6", "route", "replace", "2001:db8::1", "dev", "tun0"}},
		{name: "IPv4-mapped IPv6", host: netip.MustParseAddr("::ffff:192.0.2.1"), want: []string{"ip", "-4", "route", "replace", "192.0.2.1", "dev", "tun0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingRunner{}
			if err := New(recorder).RouteReplaceDev(test.host, "tun0"); err != nil {
				t.Fatal(err)
			}
			if len(recorder.combinedCalls) != 1 || !reflect.DeepEqual(recorder.combinedCalls[0], test.want) {
				t.Fatalf("calls = %v, want %v", recorder.combinedCalls, test.want)
			}
		})
	}
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).RouteReplaceDev(netip.MustParseAddr("1.1.1.1"), "tun0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteReplaceViaDev(t *testing.T) {
	for _, test := range []struct {
		name    string
		host    netip.Addr
		gateway netip.Addr
		want    []string
	}{
		{name: "IPv4", host: netip.MustParseAddr("1.1.1.1"), gateway: netip.MustParseAddr("10.0.0.1"), want: []string{"ip", "-4", "route", "replace", "1.1.1.1", "via", "10.0.0.1", "dev", "tun0"}},
		{name: "IPv4-mapped IPv6", host: netip.MustParseAddr("::ffff:192.0.2.1"), gateway: netip.MustParseAddr("::ffff:192.0.2.254"), want: []string{"ip", "-4", "route", "replace", "192.0.2.1", "via", "192.0.2.254", "dev", "tun0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingRunner{}
			if err := New(recorder).RouteReplaceViaDev(test.host, "tun0", test.gateway); err != nil {
				t.Fatal(err)
			}
			if len(recorder.combinedCalls) != 1 || !reflect.DeepEqual(recorder.combinedCalls[0], test.want) {
				t.Fatalf("calls = %v, want %v", recorder.combinedCalls, test.want)
			}
		})
	}
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).RouteReplaceViaDev(
			netip.MustParseAddr("1.1.1.1"),
			"tun0",
			netip.MustParseAddr("10.0.0.1"),
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid gateway", func(t *testing.T) {
		err := New(&recordingRunner{}).RouteReplaceViaDev(netip.MustParseAddr("1.1.1.1"), "tun0", netip.Addr{})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRouteDel(t *testing.T) {
	for _, test := range []struct {
		name string
		host netip.Addr
		want []string
	}{
		{name: "IPv6", host: netip.MustParseAddr("2001:db8::1"), want: []string{"ip", "-6", "route", "del", "2001:db8::1"}},
		{name: "IPv4-mapped IPv6", host: netip.MustParseAddr("::ffff:192.0.2.1"), want: []string{"ip", "-4", "route", "del", "192.0.2.1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingRunner{}
			if err := New(recorder).RouteDel(test.host); err != nil {
				t.Fatal(err)
			}
			if len(recorder.combinedCalls) != 1 || !reflect.DeepEqual(recorder.combinedCalls[0], test.want) {
				t.Fatalf("calls = %v, want %v", recorder.combinedCalls, test.want)
			}
		})
	}
	t.Run("error", func(t *testing.T) {
		err := newConfigurator(false, "output", errors.New("fail")).RouteDel(netip.MustParseAddr("1.1.1.1"))
		if err == nil {
			t.Fatal("expected error")
		}
	})
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
