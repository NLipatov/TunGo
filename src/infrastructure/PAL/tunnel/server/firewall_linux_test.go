package server

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"tungo/infrastructure/settings"
)

// ---------------------------------------------------------------------------
// enableKernelForwarding
// ---------------------------------------------------------------------------

func TestEnableKernelForwarding(t *testing.T) {
	tests := []struct {
		name    string
		sys     *TunFactoryMockSys
		ipv4    bool
		ipv6    bool
		wantErr string
	}{
		{
			name: "both_disabled_noop",
			sys:  &TunFactoryMockSys{},
			ipv4: false, ipv6: false,
			wantErr: "",
		},
		{
			name: "ipv4_already_enabled",
			sys:  &TunFactoryMockSys{},
			ipv4: true, ipv6: false,
			wantErr: "", // default output is "net.ipv4.ip_forward = 1\n"
		},
		{
			name: "ipv4_needs_write_succeeds",
			sys: &TunFactoryMockSys{
				netOutput: []byte("net.ipv4.ip_forward = 0\n"),
			},
			ipv4: true, ipv6: false,
			wantErr: "",
		},
		{
			name: "ipv4_read_error",
			sys: &TunFactoryMockSys{
				netErr: true,
			},
			ipv4: true, ipv6: false,
			wantErr: "failed to enable IPv4 packet forwarding",
		},
		{
			name: "ipv4_write_error",
			sys: &TunFactoryMockSys{
				netOutput: []byte("net.ipv4.ip_forward = 0\n"),
				wErr:      true,
			},
			ipv4: true, ipv6: false,
			wantErr: "failed to enable IPv4 packet forwarding",
		},
		{
			name: "ipv6_already_enabled",
			sys:  &TunFactoryMockSys{},
			ipv4: false, ipv6: true,
			wantErr: "", // default output is "net.ipv6.conf.all.forwarding = 1\n"
		},
		{
			name: "ipv6_needs_write_succeeds",
			sys: &TunFactoryMockSys{
				net6Output: []byte("net.ipv6.conf.all.forwarding = 0\n"),
			},
			ipv4: false, ipv6: true,
			wantErr: "",
		},
		{
			name: "ipv6_read_error",
			sys: &TunFactoryMockSys{
				net6Err: true,
			},
			ipv4: false, ipv6: true,
			wantErr: "failed to read IPv6 forwarding state",
		},
		{
			name: "ipv6_write_error",
			sys: &TunFactoryMockSys{
				net6Output: []byte("net.ipv6.conf.all.forwarding = 0\n"),
				w6Err:      true,
			},
			ipv4: false, ipv6: true,
			wantErr: "failed to enable IPv6 packet forwarding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := firewallConfigurator{
				iptables: &TunFactoryMockIPT{},
				sysctl:   tt.sys,
				mss:      &TunFactoryMockMSS{},
			}
			err := fw.enableKernelForwarding(tt.ipv4, tt.ipv6)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// configure
// ---------------------------------------------------------------------------

func TestConfigure_SuccessIPv4Only(t *testing.T) {
	ipt := &TunFactoryMockIPT{}
	mss := &TunFactoryMockMSS{}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	err := fw.configure("tun0", "eth0", baseCfg, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ipt.log.String(), "masq_on") {
		t.Fatalf("expected masquerade enable in log, got %q", ipt.log.String())
	}
	if !strings.Contains(mss.log.String(), "mss_on") {
		t.Fatalf("expected MSS install in log, got %q", mss.log.String())
	}
}

func TestConfigure_SuccessDualStack(t *testing.T) {
	ipt := &TunFactoryMockIPT{}
	mss := &TunFactoryMockMSS{}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	err := fw.configure("tun0", "eth0", baseCfgIPv6, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ipt.lastEnableMasqCIDR == "" {
		t.Fatal("expected IPv4 masquerade CIDR to be set")
	}
	if ipt.lastEnable6MasqCIDR == "" {
		t.Fatal("expected IPv6 masquerade CIDR to be set")
	}
}

func TestConfigure_IPv4MasqueradeCIDRError(t *testing.T) {
	fw := firewallConfigurator{
		iptables: &TunFactoryMockIPT{},
		sysctl:   &TunFactoryMockSys{},
		mss:      &TunFactoryMockMSS{},
	}
	// IPv4 enabled but no valid subnet in settings
	err := fw.configure("tun0", "eth0", settings.Settings{}, true, false)
	if err == nil || !strings.Contains(err.Error(), "failed to derive IPv4 NAT source subnet") {
		t.Fatalf("expected CIDR error, got %v", err)
	}
}

func TestConfigure_EnableDevMasqueradeError(t *testing.T) {
	iptBase := &TunFactoryMockIPT{}
	ipt := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: iptBase,
		errTag:            "EnableDevMasquerade",
		err:               errors.New("masq_fail"),
	}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      &TunFactoryMockMSS{},
	}
	err := fw.configure("tun0", "eth0", baseCfg, true, false)
	if err == nil || !strings.Contains(err.Error(), "failed enabling NAT") {
		t.Fatalf("expected NAT error, got %v", err)
	}
}

func TestConfigure_Enable6DevMasqueradeError_RollbacksV4(t *testing.T) {
	iptBase := &TunFactoryMockIPT{}
	ipt := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: iptBase,
		errTag:            "Enable6DevMasquerade",
		err:               errors.New("masq6_fail"),
	}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      &TunFactoryMockMSS{},
	}
	err := fw.configure("tun0", "eth0", baseCfgIPv6, true, true)
	if err == nil || !strings.Contains(err.Error(), "failed enabling IPv6 NAT") {
		t.Fatalf("expected IPv6 NAT error, got %v", err)
	}
	// IPv4 masquerade was enabled, so rollback should disable it
	if !strings.Contains(iptBase.log.String(), "masq_off") {
		t.Fatalf("expected IPv4 masquerade rollback, log=%q", iptBase.log.String())
	}
}

func TestConfigure_SetupForwardingError_RollbacksMasquerade(t *testing.T) {
	iptBase := &TunFactoryMockIPT{}
	ipt := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: iptBase,
		errTag:            "EnableForwardingFromTunToDev",
		err:               errors.New("fwd_fail"),
	}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      &TunFactoryMockMSS{},
	}
	err := fw.configure("tun0", "eth0", baseCfg, true, false)
	if err == nil || !strings.Contains(err.Error(), "failed to set up forwarding") {
		t.Fatalf("expected forwarding error, got %v", err)
	}
	// masquerade was enabled, so rollback should disable it
	if !strings.Contains(iptBase.log.String(), "masq_off") {
		t.Fatalf("expected masquerade rollback on forwarding failure, log=%q", iptBase.log.String())
	}
}

func TestConfigure_MSSInstallError_RollbacksForwardingAndMasquerade(t *testing.T) {
	iptBase := &TunFactoryMockIPT{}
	mssErr := &TunFactoryMockMSSErr{
		TunFactoryMockMSS: &TunFactoryMockMSS{},
		errTag:            "Install",
		err:               errors.New("mss_fail"),
	}
	fw := firewallConfigurator{
		iptables: iptBase,
		sysctl:   &TunFactoryMockSys{},
		mss:      mssErr,
	}
	err := fw.configure("tun0", "eth0", baseCfg, true, false)
	if err == nil || !strings.Contains(err.Error(), "failed to install MSS clamping") {
		t.Fatalf("expected MSS error, got %v", err)
	}
	logStr := iptBase.log.String()
	// Forwarding was set up, so rollback should clear it
	if !strings.Contains(logStr, "fwd_td_off") {
		t.Fatalf("expected forwarding rollback, log=%q", logStr)
	}
	// Masquerade was enabled, so rollback should disable it
	if !strings.Contains(logStr, "masq_off") {
		t.Fatalf("expected masquerade rollback, log=%q", logStr)
	}
}

// ---------------------------------------------------------------------------
// teardown
// ---------------------------------------------------------------------------

func TestTeardown_NormalWithExtIface(t *testing.T) {
	ipt := &TunFactoryMockIPT{}
	mss := &TunFactoryMockMSS{}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	fw.teardown("tun0", "eth0", baseCfg)

	logStr := ipt.log.String()
	for _, tag := range []string{"fwd_td_off", "fwd_dt_off", "fwd_tt_off", "masq_off"} {
		if !strings.Contains(logStr, tag) {
			t.Errorf("expected %q in iptables log, got %q", tag, logStr)
		}
	}
	if !strings.Contains(mss.log.String(), "mss_off") {
		t.Errorf("expected MSS remove in log, got %q", mss.log.String())
	}
}

func TestTeardown_EmptyExtIface_SkipsIptablesCleanup(t *testing.T) {
	ipt := &TunFactoryMockIPT{}
	mss := &TunFactoryMockMSS{}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	fw.teardown("tun0", "", baseCfg)

	// Forwarding from/to dev is skipped, but tun-to-tun and MSS still run
	logStr := ipt.log.String()
	if strings.Contains(logStr, "fwd_td_off") {
		t.Errorf("expected no tun-to-dev cleanup when extIface empty, log=%q", logStr)
	}
	if !strings.Contains(logStr, "fwd_tt_off") {
		t.Errorf("expected tun-to-tun cleanup even without extIface, log=%q", logStr)
	}
	if !strings.Contains(mss.log.String(), "mss_off") {
		t.Errorf("expected MSS remove, log=%q", mss.log.String())
	}
}

func TestTeardown_BenignErrorsSuppressed(t *testing.T) {
	mss := &TunFactoryMockMSSErr{
		TunFactoryMockMSS: &TunFactoryMockMSS{},
		errTag:            "Remove",
		err:               errors.New("not found, nothing to dispose"),
	}
	fw := firewallConfigurator{
		iptables: &TunFactoryMockIPTBenign{},
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	// Should not panic or produce fatal behavior; benign errors are silently ignored.
	fw.teardown("tun0", "eth0", baseCfg)
}

func TestTeardown_NonBenignErrorsLoggedNotFatal(t *testing.T) {
	mss := &TunFactoryMockMSSErr{
		TunFactoryMockMSS: &TunFactoryMockMSS{},
		errTag:            "Remove",
		err:               errors.New("permission denied"),
	}
	fw := firewallConfigurator{
		iptables: &TunFactoryMockIPTAlwaysErr{},
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	// Should not panic; non-benign errors are logged but teardown completes.
	fw.teardown("tun0", "eth0", baseCfg)
}

// ---------------------------------------------------------------------------
// unconfigure
// ---------------------------------------------------------------------------

func TestUnconfigure_DirectSuccess(t *testing.T) {
	ipt := &TunFactoryMockIPT{}
	mss := &TunFactoryMockMSS{}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	err := fw.unconfigure("tun0", "eth0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(mss.log.String(), "mss_off") {
		t.Errorf("expected MSS remove, got %q", mss.log.String())
	}
}

func TestUnconfigure_EmptyTunName_Noop(t *testing.T) {
	ipt := &TunFactoryMockIPT{}
	mss := &TunFactoryMockMSS{}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      mss,
	}
	err := fw.unconfigure("", "eth0")
	if err != nil {
		t.Fatalf("expected nil for empty tunName, got %v", err)
	}
	if mss.log.Len() != 0 {
		t.Errorf("expected no MSS calls for empty tunName, got %q", mss.log.String())
	}
}

func TestUnconfigure_ClearForwardingError_Propagated(t *testing.T) {
	ipt := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: &TunFactoryMockIPT{},
		errTag:            "DisableForwardingFromTunToDev",
		err:               errors.New("clear_err"),
	}
	fw := firewallConfigurator{
		iptables: ipt,
		sysctl:   &TunFactoryMockSys{},
		mss:      &TunFactoryMockMSS{},
	}
	err := fw.unconfigure("tun0", "eth0")
	if err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Fatalf("expected clearForwarding error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// masqueradeCIDR4
// ---------------------------------------------------------------------------

func TestMasqueradeCIDR4(t *testing.T) {
	tests := []struct {
		name    string
		cfg     settings.Settings
		want    string
		wantErr string
	}{
		{
			name: "valid_ipv4_subnet",
			cfg:  baseCfg,
			want: "10.0.0.0/30",
		},
		{
			name:    "no_subnet",
			cfg:     settings.Settings{},
			wantErr: "no IPv4 subnet configured",
		},
		{
			name: "ipv6_in_ipv4_field",
			cfg: settings.Settings{
				Addressing: settings.Addressing{
					IPv4Subnet: netip.MustParsePrefix("fd00::/64"),
				},
			},
			wantErr: "no IPv4 subnet configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := masqueradeCIDR4(tt.cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
