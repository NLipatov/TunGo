package tui

import (
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	clientcfg "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"

	srv "tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
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
	_ = foreground.Enabled()
	_ = background.Enabled()
	return m.selector, m.err
}

type mockManager struct {
	confRet   *srv.Configuration
	confErr   error
	incErr    error
	injectErr error
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

func (m *mockManager) EnsureIPv6Subnets() error { return nil }
func (m *mockManager) InvalidateCache()         {}

func withServerConfiguratorHooks(
	t *testing.T,
	newGenerator func(srv.ConfigurationManager) clientConfigGenerator,
	marshalFn func(any) ([]byte, error),
	printFn func(string),
) {
	t.Helper()
	prevGenerator := newServerClientConfigGenerator
	prevMarshal := marshalServerClientConfiguration
	prevPrint := printServerClientConfiguration
	newServerClientConfigGenerator = newGenerator
	marshalServerClientConfiguration = marshalFn
	printServerClientConfiguration = printFn
	t.Cleanup(func() {
		newServerClientConfigGenerator = prevGenerator
		marshalServerClientConfiguration = prevMarshal
		printServerClientConfiguration = prevPrint
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

	printServerClientConfiguration("")
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
	var printed string
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
		func(s string) {
			printed = s
		},
	)

	sc := newServerConfigurator(m, sf)

	if err := sc.Configure(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sf.called < 2 {
		t.Fatalf("expected selector factory called at least twice (recursion), got %d", sf.called)
	}
	if printed == "" {
		t.Fatal("expected generated client configuration to be printed")
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
		printServerClientConfiguration,
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
		printServerClientConfiguration,
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
