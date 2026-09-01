package mssclamp

import (
	"errors"
	"strings"
	"testing"
)

const (
	iptablesProbe  = "iptables --version"
	ip6tablesProbe = "ip6tables -t mangle -L -n"
	nftProbe       = "nft --version"
)

type recordingRunner struct {
	calls     []string
	outputMap map[string][]byte
	errMap    map[string]error
}

func newRecordingRunner(available ...string) *recordingRunner {
	errMap := map[string]error{
		iptablesProbe:  errors.New("iptables unavailable"),
		ip6tablesProbe: errors.New("ip6tables unavailable"),
		nftProbe:       errors.New("nft unavailable"),
	}
	for _, command := range available {
		switch command {
		case "iptables":
			delete(errMap, iptablesProbe)
		case "ip6tables":
			delete(errMap, ip6tablesProbe)
		case "nft":
			delete(errMap, nftProbe)
		}
	}
	return &recordingRunner{
		outputMap: make(map[string][]byte),
		errMap:    errMap,
	}
}

func (m *recordingRunner) record(name string, args ...string) string {
	cmd := strings.Join(append([]string{name}, args...), " ")
	m.calls = append(m.calls, cmd)
	return cmd
}

func (m *recordingRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := m.record(name, args...)
	return m.outputMap[cmd], m.errMap[cmd]
}

func (m *recordingRunner) Output(name string, args ...string) ([]byte, error) {
	cmd := m.record(name, args...)
	return m.outputMap[cmd], m.errMap[cmd]
}

func (m *recordingRunner) Run(name string, args ...string) error {
	cmd := m.record(name, args...)
	return m.errMap[cmd]
}

func TestInstallPrefersIptablesWhenItCoversRequestedFamilies(t *testing.T) {
	for _, test := range []struct {
		name       string
		families   Families
		available  []string
		wantProbes []string
		wantRules  []string
		absent     []string
	}{
		{
			name:       "IPv4 only",
			families:   Families{IPv4: true},
			available:  []string{"iptables", "nft"},
			wantProbes: []string{iptablesProbe},
			wantRules:  []string{"iptables -t mangle -A OUTPUT", "iptables -t mangle -A FORWARD -o", "iptables -t mangle -A FORWARD -i"},
			absent:     []string{"ip6tables", "nft --version"},
		},
		{
			name:       "IPv6 only",
			families:   Families{IPv6: true},
			available:  []string{"ip6tables", "nft"},
			wantProbes: []string{ip6tablesProbe},
			wantRules:  []string{"ip6tables -t mangle -A OUTPUT", "ip6tables -t mangle -A FORWARD -o", "ip6tables -t mangle -A FORWARD -i"},
			absent:     []string{"iptables --version", "nft --version"},
		},
		{
			name:       "dual stack",
			families:   Families{IPv4: true, IPv6: true},
			available:  []string{"iptables", "ip6tables", "nft"},
			wantProbes: []string{iptablesProbe, ip6tablesProbe},
			wantRules: []string{
				"iptables -t mangle -A OUTPUT",
				"iptables -t mangle -A FORWARD -o",
				"iptables -t mangle -A FORWARD -i",
				"ip6tables -t mangle -A OUTPUT",
				"ip6tables -t mangle -A FORWARD -o",
				"ip6tables -t mangle -A FORWARD -i",
			},
			absent: []string{"nft --version"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := newRecordingRunner(test.available...)
			if err := NewManager(cmd).Install("tun0", test.families); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			calls := strings.Join(cmd.calls, "\n")
			for _, want := range append(test.wantProbes, test.wantRules...) {
				if !strings.Contains(calls, want) {
					t.Errorf("calls do not contain %q:\n%s", want, calls)
				}
			}
			for _, absent := range test.absent {
				if strings.Contains(calls, absent) {
					t.Errorf("calls unexpectedly contain %q:\n%s", absent, calls)
				}
			}
		})
	}
}

func TestInstallFallsBackToNftWhenIptablesCannotCoverRequestedFamilies(t *testing.T) {
	for _, test := range []struct {
		name       string
		families   Families
		available  []string
		wantProbes []string
	}{
		{
			name:       "IPv4",
			families:   Families{IPv4: true},
			available:  []string{"nft"},
			wantProbes: []string{iptablesProbe, nftProbe},
		},
		{
			name:       "IPv6",
			families:   Families{IPv6: true},
			available:  []string{"nft"},
			wantProbes: []string{ip6tablesProbe, nftProbe},
		},
		{
			name:       "dual stack without ip6tables",
			families:   Families{IPv4: true, IPv6: true},
			available:  []string{"iptables", "nft"},
			wantProbes: []string{iptablesProbe, ip6tablesProbe, nftProbe},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := newRecordingRunner(test.available...)
			if err := NewManager(cmd).Install("tun0", test.families); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			calls := strings.Join(cmd.calls, "\n")
			for _, want := range test.wantProbes {
				if !strings.Contains(calls, want) {
					t.Errorf("calls do not contain %q:\n%s", want, calls)
				}
			}
			if !strings.Contains(calls, "nft add table inet "+nftTableName("tun0")) {
				t.Errorf("nft rules were not installed:\n%s", calls)
			}
			if !strings.Contains(calls, "tcp flags syn / syn,rst tcp option maxseg size set rt mtu") {
				t.Errorf("nft MSS rule is invalid:\n%s", calls)
			}
			if strings.Contains(calls, "iptables -t mangle -A") || strings.Contains(calls, "ip6tables -t mangle -A") {
				t.Errorf("iptables rules were mixed with nft rules:\n%s", calls)
			}
		})
	}
}

func TestNftTablesAreScopedToTun(t *testing.T) {
	cmd := newRecordingRunner("nft")
	manager := NewManager(cmd)
	for _, tunName := range []string{"tun-0", "tun-1"} {
		if err := manager.Install(tunName, Families{IPv4: true}); err != nil {
			t.Fatalf("Install(%q) error = %v", tunName, err)
		}
	}

	firstTable := nftTableName("tun-0")
	secondTable := nftTableName("tun-1")
	if firstTable == secondTable {
		t.Fatalf("both TUNs use table %q", firstTable)
	}
	calls := strings.Join(cmd.calls, "\n")
	for _, table := range []string{firstTable, secondTable} {
		if !strings.Contains(calls, "nft add table inet "+table) {
			t.Errorf("table %q was not created:\n%s", table, calls)
		}
		if count := strings.Count(calls, "nft delete table inet "+table); count != 1 {
			t.Errorf("table %q deleted %d times, want 1:\n%s", table, count, calls)
		}
	}

	cmd.calls = nil
	if err := manager.Remove("tun-0"); err != nil {
		t.Fatalf("Remove(%q) error = %v", "tun-0", err)
	}
	calls = strings.Join(cmd.calls, "\n")
	if !strings.Contains(calls, "nft delete table inet "+firstTable) {
		t.Errorf("selected TUN table was not deleted:\n%s", calls)
	}
	if strings.Contains(calls, "nft delete table inet "+secondTable) {
		t.Errorf("another TUN table was deleted:\n%s", calls)
	}
}

func TestInstallRejectsMissingFamilies(t *testing.T) {
	cmd := newRecordingRunner("iptables", "ip6tables", "nft")
	if err := NewManager(cmd).Install("tun0", Families{}); err == nil {
		t.Fatal("Install() error = nil")
	}
	if len(cmd.calls) != 0 {
		t.Fatalf("commands executed for empty families: %v", cmd.calls)
	}
}

func TestInstallFailsWhenNoBackendCoversRequestedFamilies(t *testing.T) {
	cmd := newRecordingRunner("iptables")
	err := NewManager(cmd).Install("tun0", Families{IPv4: true, IPv6: true})
	if err == nil || !strings.Contains(err.Error(), "requested IP families") {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestInstallReturnsIptablesRuleError(t *testing.T) {
	cmd := newRecordingRunner("iptables")
	failedCommand := "iptables -t mangle -A FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu"
	cmd.errMap[failedCommand] = errors.New("permission denied")

	err := NewManager(cmd).Install("tun0", Families{IPv4: true})
	if err == nil || !strings.Contains(err.Error(), "FORWARD") {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestRemoveCleansEveryAvailableBackend(t *testing.T) {
	cmd := newRecordingRunner("iptables", "ip6tables", "nft")
	missingRule := "iptables -t mangle -D OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu"
	cmd.errMap[missingRule] = errors.New("Bad rule (does a matching rule exist in that chain?)")
	missingTable := "nft delete table inet " + nftTableName("tun0")
	cmd.outputMap[missingTable] = []byte("No such table")
	cmd.errMap[missingTable] = errors.New("delete failed")
	legacyTable := "nft delete table inet " + legacyNftTable
	cmd.outputMap[legacyTable] = []byte("No such table")
	cmd.errMap[legacyTable] = errors.New("delete failed")

	if err := NewManager(cmd).Remove("tun0"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	calls := strings.Join(cmd.calls, "\n")
	for _, want := range []string{
		iptablesProbe,
		"iptables -t mangle -D FORWARD -i",
		ip6tablesProbe,
		"ip6tables -t mangle -D FORWARD -i",
		nftProbe,
		missingTable,
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("calls do not contain %q:\n%s", want, calls)
		}
	}
}

func TestRemoveReturnsRealErrorsAfterTryingEveryBackend(t *testing.T) {
	cmd := newRecordingRunner("iptables", "ip6tables", "nft")
	v4Failure := "iptables -t mangle -D OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu"
	v6Failure := "ip6tables -t mangle -D OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu"
	nftFailure := "nft delete table inet " + nftTableName("tun0")
	cmd.errMap[v4Failure] = errors.New("v4 denied")
	cmd.errMap[v6Failure] = errors.New("v6 denied")
	cmd.errMap[nftFailure] = errors.New("nft denied")

	err := NewManager(cmd).Remove("tun0")
	if err == nil {
		t.Fatal("Remove() error = nil")
	}
	for _, want := range []string{"v4 denied", "v6 denied", "nft denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Remove() error = %v, want %q", err, want)
		}
	}
	if calls := strings.Join(cmd.calls, "\n"); !strings.Contains(calls, "ip6tables -t mangle -D FORWARD -i") || !strings.Contains(calls, nftFailure) {
		t.Fatalf("cleanup stopped after first error:\n%s", calls)
	}
}

func TestRemoveFailsWhenNoCleanupBackendIsAvailable(t *testing.T) {
	cmd := newRecordingRunner()
	if err := NewManager(cmd).Remove("tun0"); err == nil {
		t.Fatal("Remove() error = nil")
	}
}
