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
			failover := Failover{configurations: configurations}
			// Rotation starts at the first named profile, even if it is already active.
			for i, want := range tc.sequence {
				if err := failover.Failover(); err != nil {
					t.Fatalf("Failover() call %d: %v", i+1, err)
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
			failover := Failover{configurations: configurations}
			if err := failover.Failover(); !errors.Is(err, ErrNoFailoverCandidates) {
				t.Fatalf("Failover() error = %v, want ErrNoFailoverCandidates", err)
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

func TestFailoverAfterNamedConfigurationDeleted(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a", "b", "c")
	failover := Failover{configurations: configurations}
	for range 2 {
		if err := failover.Failover(); err != nil {
			t.Fatal(err)
		}
	}
	if err := configurations.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if err := failover.Failover(); err != nil {
		t.Fatal(err)
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
	failover := Failover{configurations: configurations}
	var syntaxError *json.SyntaxError
	if err := failover.Failover(); !errors.As(err, &syntaxError) {
		t.Fatalf("Failover() error = %v, want JSON syntax error", err)
	}
	if err := failover.Failover(); err != nil {
		t.Fatalf("Failover() after invalid profile: %v", err)
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
	configurations := &Configurations{
		activePath: filepath.Join(t.TempDir(), "client_configuration.json"),
	}
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
