package bubble_tea

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"tungo/internal/config/settings"
	"tungo/internal/mode"

	tea "charm.land/bubbletea/v2"
)

func TestRuntimeDashboardRefreshesRuntimeInfoOnTick(t *testing.T) {
	for _, graphEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("graph enabled %v", graphEnabled), func(t *testing.T) {
			var info RuntimeInfo
			ready := false
			m := NewRuntimeDashboard(t.Context(), RuntimeDashboardOptions{
				Mode:        mode.Client,
				Ready:       func() bool { return ready },
				RuntimeInfo: func() RuntimeInfo { return info },
			}, testSettings())
			m.preferences.ShowDataplaneGraph = graphEnabled
			m.preferences.ShowDataplaneStats = false
			if view := m.View().Content; strings.Contains(view, "Protocol:") || strings.Contains(view, "Server Address:") {
				t.Fatalf("runtime information shown before settings were loaded: %s", view)
			}

			info = RuntimeInfo{
				Protocol: settings.TCP,
				Endpoints: []EndpointInfo{{
					Protocol:   settings.TCP,
					Server:     settings.Host{IPv4: "192.0.2.1", IPv6: "2001:db8::1"},
					TunnelIPv4: netip.MustParseAddr("10.0.0.2"),
					TunnelIPv6: netip.MustParseAddr("fd00::2"),
				}},
			}
			ready = true
			updated, cmd := m.Update(runtimeTickMsg{seq: m.tickSeq})
			if cmd == nil {
				t.Fatal("tick did not schedule the next update")
			}
			m = updated.(RuntimeDashboard)
			for _, want := range []string{"Status: Connected", "Protocol: TCP", "192.0.2.1", "2001:db8::1", "10.0.0.2", "fd00::2"} {
				if view := m.View().Content; !strings.Contains(view, want) {
					t.Fatalf("first connection view missing %q: %s", want, view)
				}
			}

			info = RuntimeInfo{
				Protocol: settings.UDP,
				Endpoints: []EndpointInfo{{
					Protocol:   settings.UDP,
					Server:     settings.Host{IPv4: "198.51.100.10"},
					TunnelIPv4: netip.MustParseAddr("10.1.0.2"),
				}},
			}
			updated, _ = m.Update(runtimeTickMsg{seq: m.tickSeq - 1})
			m = updated.(RuntimeDashboard)
			if view := m.View().Content; !strings.Contains(view, "192.0.2.1") {
				t.Fatalf("obsolete tick changed runtime information: %s", view)
			}
			updated, cmd = m.Update(runtimeTickMsg{seq: m.tickSeq})
			if cmd == nil {
				t.Fatal("tick after failover did not schedule the next update")
			}
			m = updated.(RuntimeDashboard)
			view := m.View().Content
			for _, want := range []string{"Status: Connected", "Protocol: UDP", "198.51.100.10", "10.1.0.2"} {
				if !strings.Contains(view, want) {
					t.Fatalf("failover view missing %q: %s", want, view)
				}
			}
			for _, stale := range []string{"Protocol: TCP", "192.0.2.1", "2001:db8::1", "10.0.0.2", "fd00::2"} {
				if strings.Contains(view, stale) {
					t.Fatalf("failover view still contains %q: %s", stale, view)
				}
			}
		})
	}
}

func TestRuntimeDashboardRefreshesRuntimeInfoWhenReturningToDataplane(t *testing.T) {
	for _, tc := range []struct {
		name   string
		screen runtimeDashboardScreen
		key    rune
	}{
		{name: "escape from settings", screen: runtimeScreenSettings, key: tea.KeyEscape},
		{name: "escape from logs", screen: runtimeScreenLogs, key: tea.KeyEscape},
		{name: "tab from logs", screen: runtimeScreenLogs, key: tea.KeyTab},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := RuntimeInfo{Endpoints: []EndpointInfo{{Server: settings.Host{IPv4: "192.0.2.1"}}}}
			m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
				RuntimeInfo: func() RuntimeInfo { return info },
			}, testSettings())
			m.screen = tc.screen
			info = RuntimeInfo{Endpoints: []EndpointInfo{{Server: settings.Host{IPv4: "198.51.100.10"}}}}
			updated, cmd := m.Update(tea.KeyPressMsg{Code: tc.key})
			m = updated.(RuntimeDashboard)
			if m.screen != runtimeScreenDataplane || cmd == nil {
				t.Fatal("returning to dataplane did not restart updates")
			}
			view := m.View().Content
			if !strings.Contains(view, "198.51.100.10") || strings.Contains(view, "192.0.2.1") {
				t.Fatalf("runtime information was not refreshed on return: %s", view)
			}
		})
	}
}
