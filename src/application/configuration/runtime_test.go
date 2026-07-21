package configuration

import (
	"testing"

	"tungo/infrastructure/settings"
)

func TestClientRuntimeConfigurationActiveSettings(t *testing.T) {
	configuration := ClientRuntimeConfiguration{
		TCPSettings: settings.Settings{Addressing: settings.Addressing{TunName: "tcp0"}},
		UDPSettings: settings.Settings{Addressing: settings.Addressing{TunName: "udp0"}},
		WSSettings:  settings.Settings{Addressing: settings.Addressing{TunName: "ws0"}},
	}

	for _, test := range []struct {
		protocol settings.Protocol
		want     string
	}{
		{protocol: settings.TCP, want: "tcp0"},
		{protocol: settings.UDP, want: "udp0"},
		{protocol: settings.WS, want: "ws0"},
		{protocol: settings.WSS, want: "ws0"},
	} {
		t.Run(test.protocol.String(), func(t *testing.T) {
			configuration.Protocol = test.protocol
			got, err := configuration.ActiveSettings()
			if err != nil {
				t.Fatalf("ActiveSettings() error = %v", err)
			}
			if got.TunName != test.want {
				t.Fatalf("ActiveSettings().TunName = %q, want %q", got.TunName, test.want)
			}
		})
	}
}

func TestClientRuntimeConfigurationActiveSettingsRejectsUnknownProtocol(t *testing.T) {
	configuration := ClientRuntimeConfiguration{Protocol: settings.UNKNOWN}

	if _, err := configuration.ActiveSettings(); err == nil {
		t.Fatal("expected unsupported protocol error")
	}
}

func TestServerRuntimeConfigurationSettings(t *testing.T) {
	configuration := ServerRuntimeConfiguration{
		TCPSettings: settings.Settings{Addressing: settings.Addressing{TunName: "tcp0"}},
		UDPSettings: settings.Settings{Addressing: settings.Addressing{TunName: "udp0"}},
		WSSettings:  settings.Settings{Addressing: settings.Addressing{TunName: "ws0"}},
		EnableTCP:   true,
		EnableWS:    true,
	}

	all := configuration.AllSettings()
	if len(all) != 3 || all[0].TunName != "tcp0" || all[1].TunName != "udp0" || all[2].TunName != "ws0" {
		t.Fatalf("AllSettings() = %+v", all)
	}

	enabled := configuration.EnabledSettings()
	if len(enabled) != 2 || enabled[0].TunName != "tcp0" || enabled[1].TunName != "ws0" {
		t.Fatalf("EnabledSettings() = %+v", enabled)
	}

	configuration.EnableTCP = false
	configuration.EnableUDP = true
	configuration.EnableWS = false
	enabled = configuration.EnabledSettings()
	if len(enabled) != 1 || enabled[0].TunName != "udp0" {
		t.Fatalf("EnabledSettings() = %+v", enabled)
	}
}
