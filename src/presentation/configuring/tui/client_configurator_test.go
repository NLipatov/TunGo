package tui

import (
	"errors"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

type clientConfiguratorMockSelector struct {
	selected string
	err      error
}

func (m *clientConfiguratorMockSelector) SelectOne() (string, error) {
	return m.selected, m.err
}

type clientConfiguratorMockSelectorFactory struct {
	selector selector.Selector
	err      error
}

func (m *clientConfiguratorMockSelectorFactory) NewTuiSelector(
	_ string, _ []string,
	_ value_objects.Color, _ value_objects.Color,
) (selector.Selector, error) {
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
	textInput text_input.TextInput
	err       error
}

func (m *clientConfiguratorMockTextInputFactory) NewTextInput(_ string) (text_input.TextInput, error) {
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
	textArea text_area.TextArea
	err      error
}

func (m *clientConfiguratorMockTextAreaFactory) NewTextArea(_ string) (text_area.TextArea, error) {
	return m.textArea, m.err
}

type clientConfiguratorMockCreator struct {
	err error
}

func (m *clientConfiguratorMockCreator) Create(_ clientConfiguration.Configuration, _ string) error {
	return m.err
}

func makeCC(
	selectorFactory selector.Factory,
	textInputFactory text_input.TextInputFactory,
	textAreaFactory text_area.TextAreaFactory,
	creator clientConfiguration.Creator,
) *clientConfigurator {
	return newClientConfigurator(nil, nil, nil, creator, selectorFactory, textInputFactory, textAreaFactory)
}

func Test_selectConf_Success(t *testing.T) {
	sf := &clientConfiguratorMockSelectorFactory{
		selector: &clientConfiguratorMockSelector{selected: "opt1"},
	}
	cc := makeCC(sf, nil, nil, nil)

	got, err := cc.selectConf(
		[]string{"opt1", "opt2"},
		"prompt",
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "opt1" {
		t.Fatalf("expected %q, got %q", "opt1", got)
	}
}

func Test_selectConf_FactoryError(t *testing.T) {
	sf := &clientConfiguratorMockSelectorFactory{err: errors.New("factory fail")}
	cc := makeCC(sf, nil, nil, nil)

	_, err := cc.selectConf(
		[]string{"x"},
		"prompt",
		value_objects.NewDefaultColor(),
		value_objects.NewDefaultColor(),
	)
	if err == nil || err.Error() != "factory fail" {
		t.Fatalf("expected factory fail, got %v", err)
	}
}

func Test_selectConf_SelectOneError(t *testing.T) {
	sf := &clientConfiguratorMockSelectorFactory{
		selector: &clientConfiguratorMockSelector{err: errors.New("select fail")},
	}
	cc := makeCC(sf, nil, nil, nil)

	_, err := cc.selectConf(
		[]string{"x"},
		"prompt",
		value_objects.NewDefaultColor(),
		value_objects.NewDefaultColor(),
	)
	if err == nil || err.Error() != "select fail" {
		t.Fatalf("expected select fail, got %v", err)
	}
}

func Test_createConf_Success(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "name"},
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: `{}`},
	}
	creator := &clientConfiguratorMockCreator{}

	cc := makeCC(nil, tif, taf, creator)
	if err := cc.createConf(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_createConf_TextInputFactoryError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{err: errors.New("input-factory fail")}
	cc := makeCC(nil, tif, nil, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-input-factory error, got nil")
	}
}

func Test_createConf_TextInputValueError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{err: errors.New("value fail")},
	}
	cc := makeCC(nil, tif, nil, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-input-value error, got nil")
	}
}

func Test_createConf_TextAreaFactoryError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n"},
	}
	taf := &clientConfiguratorMockTextAreaFactory{err: errors.New("area-factory fail")}
	cc := makeCC(nil, tif, taf, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-area-factory error, got nil")
	}
}

func Test_createConf_TextAreaValueError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n"},
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{err: errors.New("area-value fail")},
	}
	cc := makeCC(nil, tif, taf, nil)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected text-area-value error, got nil")
	}
}

func Test_createConf_ParseError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n"},
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: "{bad}"},
	}
	cc := makeCC(nil, tif, taf, &clientConfiguratorMockCreator{})

	if err := cc.createConf(); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func Test_createConf_CreatorError(t *testing.T) {
	tif := &clientConfiguratorMockTextInputFactory{
		textInput: &clientConfiguratorMockTextInput{value: "n"},
	}
	taf := &clientConfiguratorMockTextAreaFactory{
		textArea: &clientConfiguratorMockTextArea{value: `{}`},
	}
	creator := &clientConfiguratorMockCreator{err: errors.New("create fail")}
	cc := makeCC(nil, tif, taf, creator)

	if err := cc.createConf(); err == nil {
		t.Fatal("expected creator error, got nil")
	}
}
