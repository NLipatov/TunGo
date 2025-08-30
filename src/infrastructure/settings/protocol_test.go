package settings

import (
	"encoding/json"
	"testing"
)

func TestProtocol_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		value   Protocol
		want    string
		wantErr bool
	}{
		{"TCP", TCP, `"TCP"`, false},
		{"UDP", UDP, `"UDP"`, false},
		{"WS", WS, `"WS"`, false},
		{"UNKNOWN is invalid for marshal", UNKNOWN, ``, true},
		{"invalid enum", Protocol(42), ``, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.value.MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(data) != tt.want {
				t.Errorf("got %s, want %s", data, tt.want)
			}
		})
	}
}

func TestProtocol_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    Protocol
		wantErr bool
	}{
		{"tcp lowercase", `"tcp"`, TCP, false},
		{"TCP uppercase", `"TCP"`, TCP, false},
		{"Udp mixed", `"uDp"`, UDP, false},
		{"ws lowercase", `"ws"`, WS, false},
		{"WS uppercase", `"WS"`, WS, false},
		{"Ws mixed", `"wS"`, WS, false},
		{"invalid value", `"SCTP"`, UNKNOWN, true},
		{"non-string", `123`, UNKNOWN, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p Protocol // zero value is UNKNOWN
			err := p.UnmarshalJSON([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				// On error, value should remain unchanged (UNKNOWN).
				if p != UNKNOWN {
					t.Fatalf("on error, protocol must remain UNKNOWN, got %v", p)
				}
				return
			}
			if p != tc.want {
				t.Errorf("got %v, want %v", p, tc.want)
			}
		})
	}
}

func TestProtocolJSON_RoundTrip(t *testing.T) {
	for _, orig := range []Protocol{TCP, UDP, WS} {
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("Marshal %v: %v", orig, err)
		}
		var p Protocol
		if err := json.Unmarshal(data, &p); err != nil {
			t.Fatalf("Unmarshal %s: %v", data, err)
		}
		if p != orig {
			t.Errorf("round-trip %v -> %v", orig, p)
		}
	}
}

func TestProtocol_String(t *testing.T) {
	tests := []struct {
		val  Protocol
		want string
	}{
		{UNKNOWN, "UNKNOWN"},
		{TCP, "TCP"},
		{UDP, "UDP"},
		{WS, "WS"},
		{Protocol(99), "invalid protocol"},
	}
	for _, tt := range tests {
		if got := tt.val.String(); got != tt.want {
			t.Errorf("String(%d)=%q, want %q", tt.val, got, tt.want)
		}
	}
}
