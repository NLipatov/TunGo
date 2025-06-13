package tui

import (
	"errors"
	"reflect"
	"testing"

	"tungo/presentation/configuring/tui/components"
)

type serverConfiguratorMockSelector struct {
	selected string
	err      error
}

func (m *serverConfiguratorMockSelector) SelectOne() (string, error) {
	return m.selected, m.err
}

type serverConfiguratorMockSelectorFactory struct {
	selector   components.Selector
	err        error
	gotLabel   string
	gotOptions []string
}

func (m *serverConfiguratorMockSelectorFactory) NewTuiSelector(label string, options []string) (components.Selector, error) {
	m.gotLabel = label
	m.gotOptions = append([]string(nil), options...)
	return m.selector, m.err
}

func Test_selectOption_Success(t *testing.T) {
	sel := &serverConfiguratorMockSelector{selected: startServerOption, err: nil}
	sf := &serverConfiguratorMockSelectorFactory{selector: sel, err: nil}

	sc := newServerConfigurator(nil, sf)
	got, err := sc.selectOption()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
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
}

func Test_selectOption_FactoryError(t *testing.T) {
	sf := &serverConfiguratorMockSelectorFactory{selector: nil, err: errors.New("factory fail")}
	sc := newServerConfigurator(nil, sf)
	if _, err := sc.selectOption(); err == nil {
		t.Fatal("expected factory error, got nil")
	}
}

func Test_selectOption_SelectOneError(t *testing.T) {
	sel := &serverConfiguratorMockSelector{selected: "", err: errors.New("select fail")}
	sf := &serverConfiguratorMockSelectorFactory{selector: sel, err: nil}
	sc := newServerConfigurator(nil, sf)
	if _, err := sc.selectOption(); err == nil {
		t.Fatal("expected select-one error, got nil")
	}
}
