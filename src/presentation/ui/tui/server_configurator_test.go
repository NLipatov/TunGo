package tui

import (
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	clientcfg "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"

	srv "tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/value_objects"
)

type mockClientConfGenerator struct {
	conf *clientcfg.Configuration
	err  error
}

func (m mockClientConfGenerator) Generate() (*clientcfg.Configuration, error) {
	return m.conf, m.err
}

type queueSelector struct {
	options []string
	errs    []error
	idx     int
}

func mustHost(raw string) settings.Host {
	h, err := settings.NewHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

func mustPrefix(raw string) netip.Prefix {
	return netip.MustParsePrefix(raw)
}

func mustAddr(raw string) netip.Addr {
	return netip.MustParseAddr(raw)
}

func (m *queueSelector) SelectOne() (string, error) {
	if m.idx >= len(m.options) {
		if len(m.options) == 0 {
			return "", errors.New("no options in queue")
		}
		return m.options[len(m.options)-1], nil
	}
	opt := m.options[m.idx]
	var err error
	if m.idx < len(m.errs) {
		err = m.errs[m.idx]
	}
	m.idx++
	return opt, err
}

type mockSelectorFactory struct {
	selector   selector.Selector
	err        error
	gotLabel   string
	gotOptions []string
	labels     []string
	options    [][]string
	called     int
}

func (m *mockSelectorFactory) NewTuiSelector(
	label string,
	options []string,
	foreground value_objects.Color,
	background value_objects.Color,
) (selector.Selector, error) {
	m.called++
	m.gotLabel = label
	m.gotOptions = append([]string(nil), options...)
	m.labels = append(m.labels, label)
	m.options = append(m.options, append([]string(nil), options...))
	_ = foreground.Enabled()
	_ = background.Enabled()
	return m.selector, m.err
}

type mockManager struct {
	confRet             *srv.Configuration
	confErr             error
	incErr              error
	injectErr           error
	setAllowedErr       error
	setAllowedCalls     int
	lastSetClientID     int
	lastSetClientEnable bool
	removeAllowedErr    error
	removeAllowedCalls  int
	lastRemovedClientID int
}

func (m *mockManager) Configuration() (*srv.Configuration, error) {
	return m.confRet, m.confErr
}

func (m *mockManager) IncrementClientCounter() error {
	return m.incErr
}

func (m *mockManager) InjectX25519Keys(_, _ []byte) error {
	return m.injectErr
}

func (m *mockManager) AddAllowedPeer(_ srv.AllowedPeer) error {
	return nil
}

func (m *mockManager) ListAllowedPeers() ([]srv.AllowedPeer, error) {
	if m.confErr != nil {
		return nil, m.confErr
	}
	if m.confRet == nil {
		return nil, nil
	}
	peers := make([]srv.AllowedPeer, len(m.confRet.AllowedPeers))
	copy(peers, m.confRet.AllowedPeers)
	return peers, nil
}

func (m *mockManager) SetAllowedPeerEnabled(clientID int, enabled bool) error {
	m.setAllowedCalls++
	m.lastSetClientID = clientID
	m.lastSetClientEnable = enabled
	if m.setAllowedErr != nil {
		return m.setAllowedErr
	}
	if m.confRet == nil {
		return nil
	}
	for i := range m.confRet.AllowedPeers {
		if m.confRet.AllowedPeers[i].ClientID == clientID {
			m.confRet.AllowedPeers[i].Enabled = enabled
			break
		}
	}
	return nil
}

func (m *mockManager) RemoveAllowedPeer(clientID int) error {
	m.removeAllowedCalls++
	m.lastRemovedClientID = clientID
	if m.removeAllowedErr != nil {
		return m.removeAllowedErr
	}
	if m.confRet == nil {
		return nil
	}
	for i := range m.confRet.AllowedPeers {
		if m.confRet.AllowedPeers[i].ClientID == clientID {
			m.confRet.AllowedPeers = append(m.confRet.AllowedPeers[:i], m.confRet.AllowedPeers[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockManager) EnsureIPv6Subnets() error { return nil }
func (m *mockManager) InvalidateCache()         {}

func withServerConfiguratorHooks(
	t *testing.T,
	newGenerator func(srv.ConfigurationManager) clientConfigGenerator,
	marshalFn func(any) ([]byte, error),
	clipboardFn func(string) error,
) {
	t.Helper()
	prevGenerator := newServerClientConfigGenerator
	prevMarshal := marshalServerClientConfiguration
	prevClipboard := writeServerClientConfigurationClipboard
	newServerClientConfigGenerator = newGenerator
	marshalServerClientConfiguration = marshalFn
	writeServerClientConfigurationClipboard = clipboardFn
	t.Cleanup(func() {
		newServerClientConfigGenerator = prevGenerator
		marshalServerClientConfiguration = prevMarshal
		writeServerClientConfigurationClipboard = prevClipboard
	})
}

func TestServerConfigurator_DefaultHooks_AreCallable(t *testing.T) {
	generator := newServerClientConfigGenerator(&mockManager{})
	if generator == nil {
		t.Fatal("expected non-nil default client config generator")
	}

	payload, err := marshalServerClientConfiguration(map[string]int{"ok": 1})
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
}

func Test_selectOption_Success(t *testing.T) {
	qsel := &queueSelector{options: []string{startServerOption}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(nil, sf)

	got, err := sc.selectOption()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != startServerOption {
		t.Fatalf("expected %q, got %q", startServerOption, got)
	}
	if sf.gotLabel != "Choose an option" {
		t.Fatalf("expected placeholder %q, got %q", "Choose an option", sf.gotLabel)
	}
	wantOpts := []string{startServerOption, addClientOption}
	wantOpts = append(wantOpts, manageClients)
	if !reflect.DeepEqual(sf.gotOptions, wantOpts) {
		t.Fatalf("expected options %v, got %v", wantOpts, sf.gotOptions)
	}
	if sf.called != 1 {
		t.Fatalf("expected factory called once, got %d", sf.called)
	}
}

func Test_selectOption_FactoryError(t *testing.T) {
	sf := &mockSelectorFactory{err: errors.New("factory fail")}
	sc := newServerConfigurator(nil, sf)

	if _, err := sc.selectOption(); err == nil {
		t.Fatal("expected factory error, got nil")
	}
}

func Test_selectOption_SelectOneError(t *testing.T) {
	qsel := &queueSelector{options: []string{""}, errs: []error{errors.New("select fail")}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(nil, sf)

	if _, err := sc.selectOption(); err == nil {
		t.Fatal("expected select-one error, got nil")
	}
}

func Test_Configure_StartServerOption(t *testing.T) {
	qsel := &queueSelector{options: []string{startServerOption}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(&mockManager{}, sf)

	if err := sc.Configure(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func Test_Configure_AddClientOption_Success_ThenExit(t *testing.T) {
	qsel := &queueSelector{options: []string{addClientOption, startServerOption}}
	sf := &mockSelectorFactory{selector: qsel}
	m := &mockManager{}
	var copiedConfig string
	withServerConfiguratorHooks(
		t,
		func(srv.ConfigurationManager) clientConfigGenerator {
			return mockClientConfGenerator{
				conf: &clientcfg.Configuration{ClientID: 1},
			}
		},
		func(v any) ([]byte, error) {
			return json.Marshal(v)
		},
		func(config string) error {
			copiedConfig = config
			return nil
		},
	)

	sc := newServerConfigurator(m, sf)

	if err := sc.Configure(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sf.called < 2 {
		t.Fatalf("expected selector factory called at least two times, got %d", sf.called)
	}
	if len(sf.labels) < 2 || !strings.Contains(sf.labels[1], "Client configuration copied to clipboard.") {
		t.Fatalf("expected copied notice in options label, got labels: %v", sf.labels)
	}
	if copiedConfig == "" || !strings.Contains(copiedConfig, "\"ClientID\":1") {
		t.Fatalf("expected generated client config copied to clipboard, got %q", copiedConfig)
	}
}

func Test_Configure_ManageClients_ToggleTwice_ThenBackToMenu(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{
			AllowedPeers: []srv.AllowedPeer{
				{
					Name:      "alpha",
					PublicKey: make([]byte, 32),
					Enabled:   true,
					ClientID:  7,
				},
			},
		},
	}
	labelEnabled := serverPeerOptionLabel(manager.confRet.AllowedPeers[0])
	disabledPeer := manager.confRet.AllowedPeers[0]
	disabledPeer.Enabled = false
	labelDisabled := serverPeerOptionLabel(disabledPeer)
	qsel := &queueSelector{
		options: []string{
			manageClients,
			labelEnabled,
			labelDisabled,
			"",
			startServerOption,
		},
		errs: []error{
			nil,
			nil,
			nil,
			selector.ErrNavigateBack,
			nil,
		},
	}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	if err := sc.Configure(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if manager.setAllowedCalls != 2 {
		t.Fatalf("expected two status update calls, got %d", manager.setAllowedCalls)
	}
	if manager.lastSetClientID != 7 || !manager.lastSetClientEnable {
		t.Fatalf(
			"expected client #7 enabled after second toggle, got id=%d enabled=%v",
			manager.lastSetClientID,
			manager.lastSetClientEnable,
		)
	}
}

func Test_Configure_ManageClients_NoClients_ShowsInfoAndReturns(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{AllowedPeers: nil},
	}
	qsel := &queueSelector{
		options: []string{
			manageClients,
			startServerOption,
		},
	}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	if err := sc.Configure(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	found := false
	for _, label := range sf.labels {
		if strings.Contains(label, "No clients configured yet.") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected no-clients notice in options label, got labels: %v", sf.labels)
	}
}

func Test_Configure_ManageClients_SetEnabledError(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{
			AllowedPeers: []srv.AllowedPeer{
				{
					Name:      "beta",
					PublicKey: make([]byte, 32),
					Enabled:   true,
					ClientID:  3,
				},
			},
		},
		setAllowedErr: errors.New("update failed"),
	}
	label := serverPeerOptionLabel(manager.confRet.AllowedPeers[0])
	qsel := &queueSelector{
		options: []string{manageClients, label},
	}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	err := sc.Configure()
	if err == nil || err.Error() != "failed to update client #3 status: update failed" {
		t.Fatalf("expected wrapped status update error, got %v", err)
	}
}

func Test_Configure_InvalidOption(t *testing.T) {
	qsel := &queueSelector{options: []string{"unknown"}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(&mockManager{}, sf)

	err := sc.Configure()
	if err == nil || err.Error() != "invalid option: unknown" {
		t.Fatalf("expected invalid option error, got %v", err)
	}
}

func Test_Configure_EscBack_ReturnsBackToModeSelection(t *testing.T) {
	qsel := &queueSelector{options: []string{""}, errs: []error{selector.ErrNavigateBack}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(&mockManager{}, sf)

	err := sc.Configure()
	if !errors.Is(err, ErrBackToModeSelection) {
		t.Fatalf("expected ErrBackToModeSelection, got %v", err)
	}
}

func Test_Configure_UserExit_ReturnsErrUserExit(t *testing.T) {
	qsel := &queueSelector{options: []string{""}, errs: []error{selector.ErrUserExit}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(&mockManager{}, sf)

	err := sc.Configure()
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
}

func Test_Configure_AddClientOption_GenerateError(t *testing.T) {
	qsel := &queueSelector{options: []string{addClientOption}}
	sf := &mockSelectorFactory{selector: qsel}
	withServerConfiguratorHooks(
		t,
		func(srv.ConfigurationManager) clientConfigGenerator {
			return mockClientConfGenerator{err: errors.New("generate failed")}
		},
		marshalServerClientConfiguration,
		func(string) error { return nil },
	)
	sc := newServerConfigurator(&mockManager{}, sf)
	if err := sc.Configure(); err == nil || err.Error() != "generate failed" {
		t.Fatalf("expected generate failed, got %v", err)
	}
}

func Test_Configure_AddClientOption_MarshalError(t *testing.T) {
	qsel := &queueSelector{options: []string{addClientOption}}
	sf := &mockSelectorFactory{selector: qsel}
	withServerConfiguratorHooks(
		t,
		func(srv.ConfigurationManager) clientConfigGenerator {
			return mockClientConfGenerator{conf: &clientcfg.Configuration{ClientID: 1}}
		},
		func(any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		},
		func(string) error { return nil },
	)
	sc := newServerConfigurator(&mockManager{}, sf)
	err := sc.Configure()
	if err == nil || err.Error() != "failed to marshal client configuration: marshal failed" {
		t.Fatalf("expected wrapped marshal error, got %v", err)
	}
}

func TestConfigureFromState_UnknownState(t *testing.T) {
	sc := newServerConfigurator(&mockManager{}, &mockSelectorFactory{selector: &queueSelector{}})
	err := sc.configureFromState(serverFlowState(99))
	if err == nil || err.Error() != "unknown server flow state: 99" {
		t.Fatalf("expected unknown state error, got %v", err)
	}
}

func Test_Configure_SelectError(t *testing.T) {
	qsel := &queueSelector{options: []string{""}, errs: []error{errors.New("select fail")}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(&mockManager{}, sf)

	err := sc.Configure()
	if err == nil || err.Error() != "select fail" {
		t.Fatalf("expected select fail, got %v", err)
	}
}

func Test_Configure_AddClientOption_CopyClipboardError(t *testing.T) {
	qsel := &queueSelector{options: []string{addClientOption}}
	sf := &mockSelectorFactory{selector: qsel}
	withServerConfiguratorHooks(
		t,
		func(srv.ConfigurationManager) clientConfigGenerator {
			return mockClientConfGenerator{conf: &clientcfg.Configuration{ClientID: 7}}
		},
		func(v any) ([]byte, error) {
			return json.Marshal(v)
		},
		func(string) error {
			return errors.New("clipboard down")
		},
	)
	sc := newServerConfigurator(&mockManager{}, sf)
	err := sc.Configure()
	if err == nil || err.Error() != "failed to copy client configuration to clipboard: clipboard down" {
		t.Fatalf("expected clipboard copy error, got %v", err)
	}
}

func TestServerPeerDisplayName_EmptyName(t *testing.T) {
	peer := srv.AllowedPeer{ClientID: 42, Name: ""}
	if got := serverPeerDisplayName(peer); got != "client-42" {
		t.Fatalf("expected 'client-42' for empty name, got %q", got)
	}
}

func TestServerPeerDisplayName_WhitespaceName(t *testing.T) {
	peer := srv.AllowedPeer{ClientID: 7, Name: "   "}
	if got := serverPeerDisplayName(peer); got != "client-7" {
		t.Fatalf("expected 'client-7' for whitespace name, got %q", got)
	}
}

func TestServerPeerDisplayName_NonEmptyName(t *testing.T) {
	peer := srv.AllowedPeer{ClientID: 1, Name: "alpha"}
	if got := serverPeerDisplayName(peer); got != "alpha" {
		t.Fatalf("expected 'alpha', got %q", got)
	}
}

func TestSelectManagedPeer_UnknownOptionError(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{
			AllowedPeers: []srv.AllowedPeer{
				{
					Name:      "alpha",
					PublicKey: make([]byte, 32),
					Enabled:   true,
					ClientID:  1,
				},
			},
		},
	}
	// The selector returns a label that does not match any peer
	qsel := &queueSelector{options: []string{"non-existent-label"}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	_, err := sc.selectManagedPeer()
	if err == nil || !strings.Contains(err.Error(), "unknown managed client option") {
		t.Fatalf("expected 'unknown managed client option' error, got %v", err)
	}
}

func TestSelectManagedPeer_ListError(t *testing.T) {
	manager := &mockManager{confErr: errors.New("list failed")}
	sf := &mockSelectorFactory{selector: &queueSelector{}}
	sc := newServerConfigurator(manager, sf)

	_, err := sc.selectManagedPeer()
	if err == nil || err.Error() != "list failed" {
		t.Fatalf("expected list failed error, got %v", err)
	}
}

func TestSelectManagedPeer_SelectorFactoryError(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{
			AllowedPeers: []srv.AllowedPeer{
				{Name: "a", PublicKey: make([]byte, 32), Enabled: true, ClientID: 1},
			},
		},
	}
	sf := &mockSelectorFactory{
		selector: &queueSelector{},
		err:      errors.New("factory failed"),
	}
	sc := newServerConfigurator(manager, sf)

	_, err := sc.selectManagedPeer()
	if err == nil || err.Error() != "factory failed" {
		t.Fatalf("expected factory failed, got %v", err)
	}
}

func TestConfigure_ManageClients_UserExitFromPeerSelection(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{
			AllowedPeers: []srv.AllowedPeer{
				{Name: "a", PublicKey: make([]byte, 32), Enabled: true, ClientID: 1},
			},
		},
	}
	label := serverPeerOptionLabel(manager.confRet.AllowedPeers[0])
	qsel := &queueSelector{
		options: []string{manageClients, label},
		errs:    []error{nil, selector.ErrUserExit},
	}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	err := sc.Configure()
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit from manage peer selection, got %v", err)
	}
}

func Test_Configure_AddClientOption_BackFromUpdatedMenu_ReturnsToModeSelection(t *testing.T) {
	qsel := &queueSelector{
		options: []string{addClientOption, ""},
		errs:    []error{nil, selector.ErrNavigateBack},
	}
	sf := &mockSelectorFactory{selector: qsel}
	withServerConfiguratorHooks(
		t,
		func(srv.ConfigurationManager) clientConfigGenerator {
			return mockClientConfGenerator{conf: &clientcfg.Configuration{ClientID: 3}}
		},
		func(v any) ([]byte, error) {
			return json.Marshal(v)
		},
		func(string) error { return nil },
	)

	sc := newServerConfigurator(&mockManager{}, sf)
	err := sc.Configure()
	if !errors.Is(err, ErrBackToModeSelection) {
		t.Fatalf("expected ErrBackToModeSelection, got %v", err)
	}
}

func TestConfigure_ManageClients_GenericErrorFromPeerSelection(t *testing.T) {
	manager := &mockManager{
		confErr: errors.New("list peers failed"),
	}
	qsel := &queueSelector{
		options: []string{manageClients},
	}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	err := sc.Configure()
	if err == nil || err.Error() != "list peers failed" {
		t.Fatalf("expected generic error propagation from manage clients flow, got %v", err)
	}
}

func TestConfigure_ManageClients_UnknownPeerOptionError(t *testing.T) {
	manager := &mockManager{
		confRet: &srv.Configuration{
			AllowedPeers: []srv.AllowedPeer{
				{Name: "x", PublicKey: make([]byte, 32), Enabled: true, ClientID: 1},
			},
		},
	}
	// The queueSelector returns "manageClients" first (menu), then an unknown
	// label that does not match any peer label, which triggers a generic error
	// from selectManagedPeer that propagates through the default branch.
	qsel := &queueSelector{
		options: []string{manageClients, "not-a-real-peer-label"},
	}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(manager, sf)

	err := sc.Configure()
	if err == nil || !strings.Contains(err.Error(), "unknown managed client option") {
		t.Fatalf("expected unknown managed client option error, got %v", err)
	}
}

func Test_Configure_AddClientOption_UserExitFromUpdatedMenu(t *testing.T) {
	qsel := &queueSelector{
		options: []string{addClientOption, ""},
		errs:    []error{nil, selector.ErrUserExit},
	}
	sf := &mockSelectorFactory{selector: qsel}
	withServerConfiguratorHooks(
		t,
		func(srv.ConfigurationManager) clientConfigGenerator {
			return mockClientConfGenerator{conf: &clientcfg.Configuration{ClientID: 3}}
		},
		func(v any) ([]byte, error) {
			return json.Marshal(v)
		},
		func(string) error { return nil },
	)

	sc := newServerConfigurator(&mockManager{}, sf)
	err := sc.Configure()
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
}
