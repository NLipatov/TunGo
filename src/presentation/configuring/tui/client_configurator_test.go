package tui

import (
	"errors"
	"testing"

	"tungo/infrastructure/PAL/client_configuration"
	"tungo/presentation/configuring/tui/components"
)

type clientConfiguratorMockSelector struct {
	selected string
	err      error
}

func (m *clientConfiguratorMockSelector) SelectOne() (string, error) {
	return m.selected, m.err
}

type clientConfiguratorMockSelectorFactory struct {
	selector components.Selector
	err      error
}

func (m *clientConfiguratorMockSelectorFactory) NewTuiSelector(_ string, _ []string) (components.Selector, error) {
	return m.selector, m.err
}

type clientConfiguratorMockTextInput struct {
	value string
	err   error
}

func (m *clientConfiguratorMockTextInput) Value() (string, error) {
	return m.value, m.err
}

type clientConfiguratorMockTextInputFactory struct {
	textInput components.TextInput
	err       error
}

func (m *clientConfiguratorMockTextInputFactory) NewTextInput(_ string) (components.TextInput, error) {
	return m.textInput, m.err
}

type clientConfiguratorMockTextArea struct {
	value string
	err   error
}

func (m *clientConfiguratorMockTextArea) Value() (string, error) {
	return m.value, m.err
}

type clientConfiguratorMockTextAreaFactory struct {
	textArea components.TextArea
	err      error
}

func (m *clientConfiguratorMockTextAreaFactory) NewTextArea(_ string) (components.TextArea, error) {
	return m.textArea, m.err
}

type clientConfiguratorMockCreator struct {
	err error
}

func (m *clientConfiguratorMockCreator) Create(_ client_configuration.Configuration, _ string) error {
	return m.err
}

func makeCC(
	selectorFactory components.SelectorFactory,
	textInputFactory components.TextInputFactory,
	textAreaFactory components.TextAreaFactory,
	creator client_configuration.Creator,
) *clientConfigurator {
	return newClientConfigurator(nil, nil, nil, creator, selectorFactory, textInputFactory, textAreaFactory)
}

func Test_selectConf_Success(t *testing.T) {
	sf := &clientConfiguratorMockSelectorFactory{
		selector: &clientConfiguratorMockSelector{selected: "opt1", err: nil},
		err:      nil,
	}
	cc := makeCC(sf, nil, nil, nil)

	got, err := cc.selectConf([]string{"opt1", "opt2"}, "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "opt1" {
		t.Fatalf("expected %q, got %q", "opt1", got)
	}
}

func Test_selectConf_FactoryError(t *testing.T) {
	sf := &clientConfiguratorMockSelectorFactory{selector: nil, err: errors.New("factory fail")}
	cc := makeCC(sf, nil, nil, nil)

	if _, err := cc.selectConf([]string{"x"}, "prompt"); err == nil {
		t.Fatal("expected factory error, got nil")
	}
}

func Test_selectConf_SelectOneError(t *testing.T) {
	sf := &clientConfiguratorMockSelectorFactory{
		selector: &clientConfiguratorMockSelector{selected: "", err: errors.New("select fail")},
		err:      nil,
	}
	cc := makeCC(sf, nil, nil, nil)

	if _, err := cc.selectConf([]string{"x"}, "prompt"); err == nil {
		t.Fatal("expected select-one error, got nil")
	}
}

func Test_createConf_Success(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "name", err: nil},
		err:       nil,
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: `{}`, err: nil},
		err:      nil,
	}
	creator := &clientConfiguratorMockCreator{err: nil}

	cc := makeCC(nil, tif, taf, creator)
	if err := cc.createConf(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_createConf_TextInputFactoryError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{nil, errors.New("input-factory fail")}
	cc := makeCC(nil, tif, nil, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-input-factory error, got nil")
	}
}

func Test_createConf_TextInputValueError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "", err: errors.New("value fail")},
		err:       nil,
	}
	cc := makeCC(nil, tif, nil, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-input-value error, got nil")
	}
}

func Test_createConf_TextAreaFactoryError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n", err: nil},
		err:       nil,
	}
	taf := &clientConfiguratorMockTextAreaFactory{nil, errors.New("area-factory fail")}
	cc := makeCC(nil, tif, taf, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-area-factory error, got nil")
	}
}

func Test_createConf_TextAreaValueError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n", err: nil},
		err:       nil,
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: "", err: errors.New("area-value fail")},
		err:      nil,
	}
	cc := makeCC(nil, tif, taf, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-area-value error, got nil")
	}
}

func Test_createConf_ParseError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n", err: nil},
		err:       nil,
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: "{bad}", err: nil},
		err:      nil,
	}
	cc := makeCC(nil, tif, taf, &clientConfiguratorMockCreator{})

	if err := cc.createConf(); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func Test_createConf_CreatorError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n", err: nil},
		err:       nil,
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: `{}`, err: nil},
		err:      nil,
	}
	creator := &clientConfiguratorMockCreator{err: errors.New("create fail")}
	cc := makeCC(nil, tif, taf, creator)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected creator error, got nil")
	}
}
