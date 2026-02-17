package tui

import (
	"encoding/json"
	"errors"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

type cfgObserverMock struct {
	results [][]string
	errs    []error
	call    int
}

func (m *cfgObserverMock) Observe() ([]string, error) {
	var res []string
	var err error
	if m.call < len(m.results) {
		res = m.results[m.call]
	}
	if m.call < len(m.errs) {
		err = m.errs[m.call]
	}
	m.call++
	return res, err
}

type cfgSelectorMock struct {
	lastSelected string
	err          error
}

func (m *cfgSelectorMock) Select(confPath string) error {
	m.lastSelected = confPath
	return m.err
}

type cfgDeleterMock struct {
	deleted []string
	err     error
}

func (m *cfgDeleterMock) Delete(p string) error {
	m.deleted = append(m.deleted, p)
	return m.err
}

type cfgCreatorMock struct {
	createdName string
	err         error
}

func (m *cfgCreatorMock) Create(_ clientConfiguration.Configuration, name string) error {
	m.createdName = name
	return m.err
}

type cfgManagerMock struct {
	results []*clientConfiguration.Configuration
	errs    []error
	call    int
}

func (m *cfgManagerMock) Configuration() (*clientConfiguration.Configuration, error) {
	var result *clientConfiguration.Configuration
	var err error
	if m.call < len(m.results) {
		result = m.results[m.call]
	}
	if m.call < len(m.errs) {
		err = m.errs[m.call]
	}
	m.call++
	return result, err
}

type queuedSelector struct {
	options []string
	errs    []error
	i       int
}

func (m *queuedSelector) SelectOne() (string, error) {
	if m.i >= len(m.options) {
		if len(m.options) == 0 {
			return "", errors.New("queue empty")
		}
		return m.options[len(m.options)-1], nil
	}
	opt := m.options[m.i]
	var err error
	if m.i < len(m.errs) {
		err = m.errs[m.i]
	}
	m.i++
	return opt, err
}

type queuedSelectorFactory struct {
	selector selector.Selector
	errs     []error
	call     int
}

func (f *queuedSelectorFactory) NewTuiSelector(
	_ string, _ []string,
	_ value_objects.Color, _ value_objects.Color,
) (selector.Selector, error) {
	var err error
	if f.call < len(f.errs) {
		err = f.errs[f.call]
	}
	f.call++
	return f.selector, err
}

type textInputMock struct {
	val string
	err error
}

func (m *textInputMock) Value() (string, error) { return m.val, m.err }

type textInputFactoryMock struct {
	ti  text_input.TextInput
	err error
}

func (m *textInputFactoryMock) NewTextInput(_ string) (text_input.TextInput, error) {
	return m.ti, m.err
}

type textAreaMock struct {
	val string
	err error
}

func (m *textAreaMock) Value() (string, error) { return m.val, m.err }

type textAreaFactoryMock struct {
	ta  text_area.TextArea
	err error
}

func (m *textAreaFactoryMock) NewTextArea(_ string) (text_area.TextArea, error) {
	return m.ta, m.err
}

func Test_Configure_ObserveError(t *testing.T) {
	obs := &cfgObserverMock{
		errs: []error{errors.New("observe fail")},
	}
	cc := newClientConfigurator(obs, nil, nil, nil, nil, nil, nil, nil)

	err := cc.Configure()
	if err == nil || err.Error() != "observe fail" {
		t.Fatalf("expected observe fail, got %v", err)
	}
}

func Test_Configure_SelectConf_FactoryError(t *testing.T) {
	obs := &cfgObserverMock{results: [][]string{{"conf1"}}}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{},
		errs:     []error{errors.New("factory fail")},
	}
	cc := newClientConfigurator(obs, nil, nil, nil, sf, nil, nil, nil)

	err := cc.Configure()
	if err == nil || err.Error() != "factory fail" {
		t.Fatalf("expected factory fail, got %v", err)
	}
}

func Test_Configure_SelectConf_SelectOneError(t *testing.T) {
	obs := &cfgObserverMock{results: [][]string{{"conf1"}}}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{""},
			errs:    []error{errors.New("select fail")},
		},
	}
	cc := newClientConfigurator(obs, nil, nil, nil, sf, nil, nil, nil)

	err := cc.Configure()
	if err == nil || err.Error() != "select fail" {
		t.Fatalf("expected select fail, got %v", err)
	}
}

func Test_Configure_DefaultSelection_Success(t *testing.T) {
	obs := &cfgObserverMock{results: [][]string{{"conf1"}}}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{"conf1"}},
	}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, nil, nil, sf, nil, nil, nil)

	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientSel.lastSelected != "conf1" {
		t.Fatalf("expected Select called with %q, got %q", "conf1", clientSel.lastSelected)
	}
}

func Test_Configure_DefaultSelection_SelectorError(t *testing.T) {
	obs := &cfgObserverMock{results: [][]string{{"confX"}}}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{"confX"}},
	}
	clientSel := &cfgSelectorMock{err: errors.New("apply fail")}

	cc := newClientConfigurator(obs, clientSel, nil, nil, sf, nil, nil, nil)

	err := cc.Configure()
	if err == nil || err.Error() != "apply fail" {
		t.Fatalf("expected apply fail, got %v", err)
	}
}

func Test_Configure_AddOption_Flow_Success(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{},
			{"newconf.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{addOption, "newconf.json"}},
	}
	tif := &textInputFactoryMock{ti: &textInputMock{val: "newconf"}}
	validCfgJSON, _ := json.Marshal(makeTestConfig())
	taf := &textAreaFactoryMock{ta: &textAreaMock{val: string(validCfgJSON)}}
	creator := &cfgCreatorMock{}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, nil, creator, sf, tif, taf, nil)

	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.createdName != "newconf" {
		t.Fatalf("expected creator to be called with name %q, got %q", "newconf", creator.createdName)
	}
	if clientSel.lastSelected != "newconf.json" {
		t.Fatalf("expected final selection %q, got %q", "newconf.json", clientSel.lastSelected)
	}
}

func Test_Configure_RemoveOption_Flow_Success(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"a.json", "b.json"},
			{"b.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{removeOption, "a.json", "b.json"}},
	}

	del := &cfgDeleterMock{}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, del, nil, sf, nil, nil, nil)

	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(del.deleted) != 1 || del.deleted[0] != "a.json" {
		t.Fatalf("expected deleted [a.json], got %v", del.deleted)
	}
	if clientSel.lastSelected != "b.json" {
		t.Fatalf("expected final selection %q, got %q", "b.json", clientSel.lastSelected)
	}
}

func Test_Configure_RemoveOption_SecondSelect_Error(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"only.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{removeOption, ""},
			errs:    []error{nil, errors.New("remove select fail")},
		},
	}
	cc := newClientConfigurator(obs, nil, &cfgDeleterMock{}, nil, sf, nil, nil, nil)

	err := cc.Configure()
	if err == nil || err.Error() != "remove select fail" {
		t.Fatalf("expected remove select fail, got %v", err)
	}
}

func Test_Configure_InvalidSelectedConfiguration_ShowsWarningAndRetries(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"broken.json", "valid.json"},
			{"broken.json", "valid.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{"broken.json", invalidConfigRetryOption, "valid.json"}},
	}
	clientSel := &cfgSelectorMock{}
	manager := &cfgManagerMock{
		errs: []error{
			errors.New("invalid client configuration (/tmp/client.json): invalid ClientID 0: must be > 0"),
			nil,
		},
	}

	cc := newClientConfigurator(obs, clientSel, nil, nil, sf, nil, nil, manager)
	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientSel.lastSelected != "valid.json" {
		t.Fatalf("expected final selection %q, got %q", "valid.json", clientSel.lastSelected)
	}
}

func Test_Configure_InvalidSelectedConfiguration_ShowDetailsThenRetry(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"broken.json", "valid.json"},
			{"broken.json", "valid.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{
				"broken.json",
				invalidConfigDetailOption,
				invalidConfigBackOption,
				invalidConfigRetryOption,
				"valid.json",
			},
		},
	}
	clientSel := &cfgSelectorMock{}
	manager := &cfgManagerMock{
		errs: []error{
			errors.New("invalid client configuration (/tmp/client.json): invalid ClientID 0: must be > 0"),
			nil,
		},
	}

	cc := newClientConfigurator(obs, clientSel, nil, nil, sf, nil, nil, manager)
	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientSel.lastSelected != "valid.json" {
		t.Fatalf("expected final selection %q, got %q", "valid.json", clientSel.lastSelected)
	}
}

func Test_Configure_SelectedConfigurationCheck_NonInvalidErrorReturned(t *testing.T) {
	obs := &cfgObserverMock{results: [][]string{{"conf1"}}}
	sf := &queuedSelectorFactory{selector: &queuedSelector{options: []string{"conf1"}}}
	clientSel := &cfgSelectorMock{}
	manager := &cfgManagerMock{errs: []error{errors.New("permission denied")}}

	cc := newClientConfigurator(obs, clientSel, nil, nil, sf, nil, nil, manager)
	err := cc.Configure()
	if err == nil || err.Error() != "permission denied" {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func Test_summarizeInvalidConfigurationError(t *testing.T) {
	msg := summarizeInvalidConfigurationError(
		errors.New("invalid client configuration (/tmp/client.json): invalid ClientID 0: must be > 0"),
	)
	if msg != "invalid ClientID 0: must be > 0" {
		t.Fatalf("unexpected summary: %q", msg)
	}
}
