package client

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFailoverRotatesNamedConfigurations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		names    []string
		sequence []int
	}{
		{name: "two", names: []string{"a", "b"}, sequence: []int{1, 2, 1}},
		{name: "three", names: []string{"a", "b", "c"}, sequence: []int{1, 2, 3, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configurations := failoverTestConfigurations(t, tc.names...)
			if err := configurations.Activate("a"); err != nil {
				t.Fatal(err)
			}
			failover := NewFailover(configurations)
			// Rotation starts at the first named profile, even if it is already active.
			for i, want := range tc.sequence {
				name, err := failover.Failover()
				if err != nil {
					t.Fatalf("Failover() call %d: %v", i+1, err)
				}
				if wantName := tc.names[want-1]; name != wantName {
					t.Fatalf("call %d: name = %q, want %q", i+1, name, wantName)
				}
				active, err := configurations.Active()
				if err != nil {
					t.Fatal(err)
				}
				if active.ClientID != want {
					t.Fatalf("call %d: active ClientID = %d, want %d", i+1, active.ClientID, want)
				}
			}
		})
	}
}

func TestFailoverRequiresTwoNamedConfigurations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
	}{
		{name: "none"},
		{name: "one", names: []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configurations := failoverTestConfigurations(t, tc.names...)
			initial := validTestConfiguration()
			initial.ClientID = 7
			writeConfiguration(t, configurations.activePath, initial)
			failover := NewFailover(configurations)
			name, err := failover.Failover()
			if name != "" || !errors.Is(err, ErrNoFailoverCandidates) {
				t.Fatalf("Failover() = %q, %v; want empty name and ErrNoFailoverCandidates", name, err)
			}
			active, err := configurations.Active()
			if err != nil {
				t.Fatal(err)
			}
			if active.ClientID != initial.ClientID {
				t.Fatalf("active ClientID = %d, want unchanged %d", active.ClientID, initial.ClientID)
			}
		})
	}
}

func TestFailoverReturnsEmptyNameOnListError(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "configurations")
	if err := os.WriteFile(directory, nil, 0600); err != nil {
		t.Fatal(err)
	}
	failover := NewFailover(NewConfigurations(directory))
	name, err := failover.Failover()
	var pathError *os.PathError
	if name != "" || !errors.As(err, &pathError) {
		t.Fatalf("Failover() = %q, %v; want empty name and directory read error", name, err)
	}
}

func TestFailoverAfterNamedConfigurationDeleted(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a", "b", "c")
	failover := NewFailover(configurations)
	for range 2 {
		if _, err := failover.Failover(); err != nil {
			t.Fatal(err)
		}
	}
	if err := configurations.Delete("a"); err != nil {
		t.Fatal(err)
	}
	name, err := failover.Failover()
	if err != nil {
		t.Fatal(err)
	}
	if name != "b" {
		t.Fatalf("Failover() name = %q, want b", name)
	}
	active, err := configurations.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.ClientID != 2 {
		t.Fatalf("active ClientID = %d, want 2", active.ClientID)
	}
}

func TestFailoverAdvancesAfterActivationError(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a", "b")
	if err := os.WriteFile(configurations.activePath+".a", []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	failover := NewFailover(configurations)
	var syntaxError *json.SyntaxError
	name, err := failover.Failover()
	if name != "a" || !errors.As(err, &syntaxError) {
		t.Fatalf("Failover() = %q, %v; want a and JSON syntax error", name, err)
	}
	name, err = failover.Failover()
	if err != nil {
		t.Fatalf("Failover() after invalid profile: %v", err)
	}
	if name != "b" {
		t.Fatalf("Failover() name after invalid profile = %q, want b", name)
	}
	active, err := configurations.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.ClientID != 2 {
		t.Fatalf("active ClientID = %d, want 2", active.ClientID)
	}
}

func failoverTestConfigurations(t *testing.T, names ...string) *Configurations {
	t.Helper()
	configurations := NewConfigurations(t.TempDir())
	for i, name := range names {
		configuration := validTestConfiguration()
		configuration.ClientID = i + 1
		data, err := json.Marshal(configuration)
		if err != nil {
			t.Fatal(err)
		}
		if err := configurations.Import(name, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	return configurations
}
