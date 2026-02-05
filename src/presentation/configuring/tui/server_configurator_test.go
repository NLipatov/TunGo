package tui

import (
	"errors"
	"reflect"
	"runtime"
	"testing"
	"tungo/infrastructure/settings"

	srv "tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

type queueSelector struct {
	options []string
	errs    []error
	idx     int
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

func (m *mockManager) InvalidateCache() {}

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
	if runtime.GOOS != "linux" {
		t.Skip("test requires Linux ip command")
	}
	qsel := &queueSelector{options: []string{addClientOption, startServerOption}}
	sf := &mockSelectorFactory{selector: qsel}

	m := &mockManager{
		confRet: &srv.Configuration{
			TCPSettings: settings.Settings{
				ConnectionIP:     "10.10.0.1",
				InterfaceIPCIDR:  "10.10.0.0/24",
				InterfaceAddress: "10.10.0.2",
			},
			UDPSettings: settings.Settings{
				ConnectionIP:     "10.10.1.1",
				InterfaceIPCIDR:  "10.10.1.1/24",
				InterfaceAddress: "10.10.1.2",
			},
			WSSettings: settings.Settings{
				ConnectionIP:     "10.10.3.1",
				InterfaceIPCIDR:  "10.10.3.1/24",
				InterfaceAddress: "10.10.3.2",
			},
			EnableTCP: true,
		},
	}

	sc := newServerConfigurator(m, sf)

	if err := sc.Configure(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sf.called < 2 {
		t.Fatalf("expected selector factory called at least twice (recursion), got %d", sf.called)
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

func Test_Configure_SelectError(t *testing.T) {
	qsel := &queueSelector{options: []string{""}, errs: []error{errors.New("select fail")}}
	sf := &mockSelectorFactory{selector: qsel}
	sc := newServerConfigurator(&mockManager{}, sf)

	err := sc.Configure()
	if err == nil || err.Error() != "select fail" {
		t.Fatalf("expected select fail, got %v", err)
	}
}
