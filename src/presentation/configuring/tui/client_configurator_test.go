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

type stagedTextAreaMock struct {
	values []string
	errs   []error
	idx    int
}

func (m *stagedTextAreaMock) Value() (string, error) {
	if m.idx >= len(m.values) && m.idx >= len(m.errs) {
		if len(m.values) > 0 {
			return m.values[len(m.values)-1], nil
		}
		if len(m.errs) > 0 {
			return "", m.errs[len(m.errs)-1]
		}
		return "", nil
	}

	cur := m.idx
	m.idx++

	var val string
	var err error
	if cur < len(m.values) {
		val = m.values[cur]
	}
	if cur < len(m.errs) {
		err = m.errs[cur]
	}
	return val, err
}

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

func Test_Configure_EscFromConfigSelection_ReturnsBackToModeSelection(t *testing.T) {
	obs := &cfgObserverMock{results: [][]string{{"conf1"}}}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{""},
			errs:    []error{selector.ErrNavigateBack},
		},
	}
	cc := newClientConfigurator(obs, &cfgSelectorMock{}, nil, nil, sf, nil, nil, nil)

	err := cc.Configure()
	if !errors.Is(err, ErrBackToModeSelection) {
		t.Fatalf("expected ErrBackToModeSelection, got %v", err)
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

func Test_Configure_AddOption_InvalidJSON_ShowsWarningAndReturnsToSelection(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"valid.json"},
			{"valid.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{addOption, "", "valid.json"},
			errs:    []error{nil, selector.ErrNavigateBack, nil},
		},
	}
	tif := &textInputFactoryMock{ti: &textInputMock{val: "broken"}}
	taf := &textAreaFactoryMock{ta: &textAreaMock{val: "{ invalid json"}}
	creator := &cfgCreatorMock{}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, nil, creator, sf, tif, taf, nil)

	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.createdName != "" {
		t.Fatalf("expected creator to not be called for invalid json, got %q", creator.createdName)
	}
	if clientSel.lastSelected != "valid.json" {
		t.Fatalf("expected final selection %q, got %q", "valid.json", clientSel.lastSelected)
	}
}

func Test_Configure_AddOption_NameInputCancelled_ReturnsToSelection(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"valid.json"},
			{"valid.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{addOption, "valid.json"}},
	}
	tif := &textInputFactoryMock{ti: &textInputMock{err: text_input.ErrCancelled}}
	taf := &textAreaFactoryMock{ta: &textAreaMock{val: ""}}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, nil, &cfgCreatorMock{}, sf, tif, taf, nil)
	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientSel.lastSelected != "valid.json" {
		t.Fatalf("expected final selection %q, got %q", "valid.json", clientSel.lastSelected)
	}
}

func Test_Configure_AddOption_TextAreaCancelled_ReturnsToNameStep(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"valid.json"},
			{"newconf.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{addOption, "newconf.json"}},
	}
	tif := &textInputFactoryMock{ti: &textInputMock{val: "newconf"}}
	validCfgJSON, _ := json.Marshal(makeTestConfig())
	taf := &textAreaFactoryMock{ta: &stagedTextAreaMock{
		values: []string{"", string(validCfgJSON)},
		errs:   []error{text_area.ErrCancelled, nil},
	}}
	creator := &cfgCreatorMock{}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, nil, creator, sf, tif, taf, nil)
	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.createdName != "newconf" {
		t.Fatalf("expected creator called with %q, got %q", "newconf", creator.createdName)
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
		selector: &queuedSelector{options: []string{removeOption, removeItemPrefix + "a.json", "b.json"}},
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

func Test_Configure_RemoveOption_Back(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"a.json", "b.json"},
			{"a.json", "b.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{removeOption, "", "b.json"},
			errs:    []error{nil, selector.ErrNavigateBack, nil},
		},
	}

	del := &cfgDeleterMock{}
	clientSel := &cfgSelectorMock{}

	cc := newClientConfigurator(obs, clientSel, del, nil, sf, nil, nil, nil)
	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(del.deleted) != 0 {
		t.Fatalf("expected nothing deleted, got %v", del.deleted)
	}
	if clientSel.lastSelected != "b.json" {
		t.Fatalf("expected final selection %q, got %q", "b.json", clientSel.lastSelected)
	}
}

func Test_Configure_InvalidSelectedConfiguration_Back_ReturnsToSelection(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"broken.json", "valid.json"},
			{"broken.json", "valid.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{"broken.json", "", "valid.json"},
			errs:    []error{nil, selector.ErrNavigateBack, nil},
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

func Test_Configure_InvalidSelectedConfiguration_Delete_RemovesBrokenConfig(t *testing.T) {
	obs := &cfgObserverMock{
		results: [][]string{
			{"broken.json", "valid.json"},
			{"valid.json"},
		},
	}
	sf := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{"broken.json", invalidConfigDeleteOption, "valid.json"}},
	}
	clientSel := &cfgSelectorMock{}
	del := &cfgDeleterMock{}
	manager := &cfgManagerMock{
		errs: []error{
			errors.New("invalid client configuration (/tmp/client.json): invalid ClientID 0: must be > 0"),
			nil,
		},
	}

	cc := newClientConfigurator(obs, clientSel, del, nil, sf, nil, nil, manager)
	if err := cc.Configure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(del.deleted) != 1 || del.deleted[0] != "broken.json" {
		t.Fatalf("expected invalid config to be deleted, got %v", del.deleted)
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
