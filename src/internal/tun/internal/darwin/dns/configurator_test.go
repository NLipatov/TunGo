//go:build darwin

package dns

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
)

type commandResult struct {
	output string
	err    error
}

type commandCall struct {
	name  string
	args  []string
	input string
}

type runnerMock struct {
	calls         []commandCall
	scutilResults []commandResult
}

func (m *runnerMock) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, commandCall{name: name, args: append([]string(nil), args...)})
	return nil, nil
}

func (m *runnerMock) CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	m.calls = append(m.calls, commandCall{name: name, args: append([]string(nil), args...), input: string(data)})
	if len(m.scutilResults) == 0 {
		return nil, errors.New("unexpected scutil call")
	}
	result := m.scutilResults[0]
	m.scutilResults = m.scutilResults[1:]
	return []byte(result.output), result.err
}

func TestConfiguratorSetsAndRestoresPrimaryDNS(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	runner := &runnerMock{scutilResults: []commandResult{
		{output: "No such key"},
		{output: propertyDictionary("PrimaryService", service)},
		{output: dnsDictionary("192.0.2.53")},
		{},
		{},
		{output: backupDictionary(service, true, "192.0.2.53")},
		{},
		{},
	}}
	configurator := New(runner)

	if err := configurator.Set([]string{"1.1.1.1", "2606:4700:4700::1111"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	serviceSetupKey := setupKey(service)
	scutilCalls := callsNamed(runner.calls, "scutil")
	for _, want := range []struct {
		index    int
		contains string
	}{
		{index: 0, contains: "show " + backupKey},
		{index: 3, contains: "get " + serviceSetupKey},
		{index: 3, contains: "d.add " + serviceProperty + " " + service},
		{index: 3, contains: "d.add " + hadSetupInitiallyProperty + " true"},
		{index: 3, contains: "set " + backupKey},
		{index: 4, contains: "d.add ServerAddresses * 1.1.1.1 2606:4700:4700::1111"},
		{index: 6, contains: "get " + backupKey},
		{index: 6, contains: "d.remove " + serviceProperty},
		{index: 6, contains: "d.remove " + hadSetupInitiallyProperty},
		{index: 6, contains: "set " + serviceSetupKey},
		{index: 7, contains: "remove " + backupKey},
	} {
		if !strings.Contains(scutilCalls[want.index].input, want.contains) {
			t.Fatalf("scutil call %d input = %q, want %q", want.index, scutilCalls[want.index].input, want.contains)
		}
	}
	if got := len(callsNamed(runner.calls, "dscacheutil")); got != 2 {
		t.Fatalf("cache flush calls = %d, want 2", got)
	}
}

func TestConfiguratorSetRestoresPreviousServiceBeforeConfiguringCurrent(t *testing.T) {
	const (
		previousService = "11111111-2222-3333-4444-555555555555"
		currentService  = "66666666-7777-8888-9999-AAAAAAAAAAAA"
	)
	runner := &runnerMock{scutilResults: []commandResult{
		{output: backupDictionary(previousService, true, "192.0.2.53")},
		{},
		{},
		{output: propertyDictionary("PrimaryService", currentService)},
		{output: dnsDictionary("198.51.100.53")},
		{},
		{},
	}}
	configurator := New(runner)

	if err := configurator.Set([]string{"1.1.1.1"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	scutilCalls := callsNamed(runner.calls, "scutil")
	if !strings.Contains(scutilCalls[1].input, "set "+setupKey(previousService)) {
		t.Fatalf("previous service restore input = %q", scutilCalls[1].input)
	}
	if !strings.Contains(scutilCalls[5].input, "d.add "+serviceProperty+" "+currentService) ||
		!strings.Contains(scutilCalls[5].input, "set "+backupKey) {
		t.Fatalf("current service backup input = %q", scutilCalls[5].input)
	}
}

func TestConfiguratorRestoresBackupFromPreviousProcess(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	runner := &runnerMock{scutilResults: []commandResult{
		{output: backupDictionary(service, true, "192.0.2.53")},
		{},
		{},
	}}
	configurator := New(runner)

	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	scutilCalls := callsNamed(runner.calls, "scutil")
	if !strings.Contains(scutilCalls[1].input, "set "+setupKey(service)) {
		t.Fatalf("restore input = %q", scutilCalls[1].input)
	}
	if got, want := scutilCalls[2].input, "remove "+backupKey+"\n"; got != want {
		t.Fatalf("backup cleanup = %q, want %q", got, want)
	}
}

func TestConfiguratorRevertWithoutBackupIsNoOp(t *testing.T) {
	runner := &runnerMock{scutilResults: []commandResult{{output: "No such key"}}}
	configurator := New(runner)

	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}
	if got := len(runner.calls); got != 1 {
		t.Fatalf("command calls = %d, want backup check only", got)
	}
}

func TestConfiguratorRemovesDNSWhenOriginalConfigDidNotExist(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	runner := &runnerMock{scutilResults: []commandResult{
		{output: "No such key"},
		{output: propertyDictionary("PrimaryService", service)},
		{output: "No such key"},
		{},
		{},
		{output: backupDictionary(service, false)},
		{},
		{},
	}}
	configurator := New(runner)

	if err := configurator.Set([]string{"1.1.1.1"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	scutilCalls := callsNamed(runner.calls, "scutil")
	if !strings.HasPrefix(scutilCalls[3].input, "d.init\n") ||
		!strings.Contains(scutilCalls[3].input, "d.add "+hadSetupInitiallyProperty+" false") {
		t.Fatalf("backup input = %q", scutilCalls[3].input)
	}
	if got, want := scutilCalls[6].input, "remove "+setupKey(service)+"\n"; got != want {
		t.Fatalf("DNS removal = %q, want %q", got, want)
	}
	if got, want := scutilCalls[7].input, "remove "+backupKey+"\n"; got != want {
		t.Fatalf("backup cleanup = %q, want %q", got, want)
	}
}

func TestConfiguratorSetDiscardsInvalidBackupAndSucceedsOnRetry(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	runner := &runnerMock{scutilResults: []commandResult{
		{output: propertyDictionary("Broken", "true")},
		{},
		{output: "No such key"},
		{output: propertyDictionary("PrimaryService", service)},
		{output: dnsDictionary("192.0.2.53")},
		{},
		{},
	}}
	configurator := New(runner)

	if err := configurator.Set([]string{"9.9.9.9"}); err == nil || !strings.Contains(err.Error(), "DNS backup is invalid") {
		t.Fatalf("first Set() error = %v, want invalid backup error", err)
	}
	if err := configurator.Set([]string{"9.9.9.9"}); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}

	scutilCalls := callsNamed(runner.calls, "scutil")
	if got, want := scutilCalls[1].input, "remove "+backupKey+"\n"; got != want {
		t.Fatalf("invalid backup cleanup = %q, want %q", got, want)
	}
	if !strings.Contains(scutilCalls[5].input, "d.add "+serviceProperty+" "+service) {
		t.Fatalf("fresh backup input = %q", scutilCalls[5].input)
	}
	if !strings.Contains(scutilCalls[6].input, "d.add ServerAddresses * 9.9.9.9") {
		t.Fatalf("DNS setup input = %q", scutilCalls[6].input)
	}
}

func TestConfiguratorUnavailableBeforeChangesDoesNotModifyDNS(t *testing.T) {
	runner := &runnerMock{scutilResults: []commandResult{{err: exec.ErrNotFound}}}
	configurator := New(runner)

	if err := configurator.Set([]string{"1.1.1.1"}); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Set() error = %v, want %v", err, exec.ErrNotFound)
	}
	if got := len(callsNamed(runner.calls, "scutil")); got != 1 {
		t.Fatalf("scutil calls = %d, want only failed backup check", got)
	}
}

func TestConfiguratorSetStopsWhenPreviousDNSCannotBeRestored(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	runner := &runnerMock{scutilResults: []commandResult{
		{output: backupDictionary(service, true, "192.0.2.53")},
		{output: "Permission denied"},
	}}
	configurator := New(runner)

	if err := configurator.Set([]string{"1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("Set() error = %v, want Permission denied", err)
	}
	if got := len(callsNamed(runner.calls, "scutil")); got != 2 {
		t.Fatalf("scutil calls = %d, want backup read and failed restore", got)
	}
}

func TestConfiguratorRejectsScutilMutationDiagnostics(t *testing.T) {
	for _, detail := range []string{
		"Permission denied",
		"Invalid argument",
		"Configuration daemon session not active",
	} {
		t.Run(detail, func(t *testing.T) {
			runner := &runnerMock{scutilResults: []commandResult{{output: detail}}}

			if err := New(runner).runScript("d.init\n"); err == nil || !strings.Contains(err.Error(), detail) {
				t.Fatalf("runScript() error = %v, want %q", err, detail)
			}
		})
	}
}

func TestConfiguratorShowRejectsUnexpectedOutput(t *testing.T) {
	for _, output := range []string{"", "Permission denied", "Invalid argument"} {
		t.Run(output, func(t *testing.T) {
			runner := &runnerMock{scutilResults: []commandResult{{output: output}}}

			if _, _, err := New(runner).show(backupKey); err == nil {
				t.Fatalf("show() error = nil, want unexpected output error for %q", output)
			}
		})
	}
}

func TestConfiguratorRemoveKeyRejectsScutilDiagnostics(t *testing.T) {
	for _, detail := range []string{"Permission denied", "Invalid argument"} {
		t.Run(detail, func(t *testing.T) {
			runner := &runnerMock{scutilResults: []commandResult{{output: detail}}}

			if err := New(runner).removeKey(backupKey); err == nil || !strings.Contains(err.Error(), detail) {
				t.Fatalf("removeKey() error = %v, want %q", err, detail)
			}
		})
	}
}

func TestConfiguratorRollsBackFailedDNSSetup(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	for _, test := range []struct {
		name          string
		restoreResult commandResult
	}{
		{name: "restore succeeds"},
		{
			name:          "restore fails",
			restoreResult: commandResult{output: "restore failed", err: errors.New("exit status 1")},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupErr := errors.New("setup failed")
			results := []commandResult{
				{output: "No such key"},
				{output: propertyDictionary("PrimaryService", service)},
				{output: dnsDictionary("192.0.2.53")},
				{},
				{output: "setup failed", err: setupErr},
				{output: backupDictionary(service, true, "192.0.2.53")},
				test.restoreResult,
			}
			if test.restoreResult.err == nil {
				results = append(results, commandResult{})
			}
			runner := &runnerMock{scutilResults: results}
			configurator := New(runner)

			err := configurator.Set([]string{"1.1.1.1"})
			if err == nil {
				t.Fatal("Set() error = nil")
			}
			if !errors.Is(err, setupErr) {
				t.Fatalf("Set() error = %v, want setup cause", err)
			}
			if test.restoreResult.err != nil && !errors.Is(err, test.restoreResult.err) {
				t.Fatalf("Set() error = %v, want restore cause", err)
			}
		})
	}
}

func TestConfiguratorKeepsBackupUntilRestoreSucceeds(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	restoreErr := errors.New("restore failed")
	backup := backupDictionary(service, true, "192.0.2.53")
	runner := &runnerMock{scutilResults: []commandResult{
		{output: backup},
		{output: "restore failed", err: restoreErr},
		{output: backup},
		{},
		{},
	}}
	configurator := New(runner)

	if err := configurator.Revert(); !errors.Is(err, restoreErr) {
		t.Fatalf("first Revert() error = %v, want %v", err, restoreErr)
	}
	if err := configurator.Revert(); err != nil {
		t.Fatalf("second Revert() error = %v", err)
	}

	scutilCalls := callsNamed(runner.calls, "scutil")
	if got, want := scutilCalls[4].input, "remove "+backupKey+"\n"; got != want {
		t.Fatalf("backup cleanup = %q, want %q", got, want)
	}
}

func TestConfiguratorIgnoresBackupRemovalFailureAfterRestore(t *testing.T) {
	const service = "11111111-2222-3333-4444-555555555555"
	removeErr := errors.New("remove failed")
	runner := &runnerMock{scutilResults: []commandResult{
		{output: backupDictionary(service, true, "192.0.2.53")},
		{},
		{output: "remove failed", err: removeErr},
	}}
	configurator := New(runner)

	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	if err := configurator.Revert(); err != nil {
		t.Fatalf("Revert() error = %v, want best-effort backup removal", err)
	}
	if !strings.Contains(logs.String(), "failed to remove restored DNS backup") ||
		!strings.Contains(logs.String(), removeErr.Error()) {
		t.Fatalf("backup removal log = %q", logs.String())
	}
}

func TestConfiguratorRevertDiscardsInvalidBackup(t *testing.T) {
	for _, test := range []struct {
		name   string
		backup string
	}{
		{name: "missing service", backup: propertyDictionary(hadSetupInitiallyProperty, "true")},
		{name: "missing initial setup marker", backup: propertyDictionary(serviceProperty, "service")},
		{name: "invalid service", backup: backupDictionary("service/nested", true)},
		{name: "invalid initial setup marker", backup: backupDictionaryWithMarker("service", "sometimes")},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &runnerMock{scutilResults: []commandResult{
				{output: test.backup},
				{},
			}}
			configurator := New(runner)

			if err := configurator.Revert(); err == nil || !strings.Contains(err.Error(), "DNS backup is invalid") {
				t.Fatalf("Revert() error = %v, want invalid backup error", err)
			}
			if got, want := runner.calls[1].input, "remove "+backupKey+"\n"; got != want {
				t.Fatalf("invalid backup cleanup = %q, want %q", got, want)
			}
		})
	}
}

func callsNamed(calls []commandCall, name string) []commandCall {
	var matched []commandCall
	for _, call := range calls {
		if call.name == name {
			matched = append(matched, call)
		}
	}
	return matched
}

func propertyDictionary(name, value string) string {
	return "<dictionary> {\n  " + name + " : " + value + "\n}"
}

func dnsDictionary(resolvers ...string) string {
	var output strings.Builder
	output.WriteString("<dictionary> {\n  ServerAddresses : <array> {\n")
	for index, resolver := range resolvers {
		output.WriteString("    " + string(rune('0'+index)) + " : " + resolver + "\n")
	}
	output.WriteString("  }\n}")
	return output.String()
}

func backupDictionary(service string, hadSetupInitially bool, resolvers ...string) string {
	marker := "false"
	if hadSetupInitially {
		marker = "true"
	}
	return backupDictionaryWithMarkerAndResolvers(service, marker, resolvers...)
}

func backupDictionaryWithMarker(service, marker string) string {
	return backupDictionaryWithMarkerAndResolvers(service, marker)
}

func backupDictionaryWithMarkerAndResolvers(service, marker string, resolvers ...string) string {
	var output strings.Builder
	output.WriteString("<dictionary> {\n")
	if len(resolvers) > 0 {
		output.WriteString("  ServerAddresses : <array> {\n")
		for index, resolver := range resolvers {
			output.WriteString("    " + string(rune('0'+index)) + " : " + resolver + "\n")
		}
		output.WriteString("  }\n")
	}
	output.WriteString("  " + serviceProperty + " : " + service + "\n")
	output.WriteString("  " + hadSetupInitiallyProperty + " : " + marker + "\n")
	output.WriteString("}")
	return output.String()
}
