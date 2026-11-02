package client

import (
	"encoding/json"
	"errors"
	"fmt"
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
			failover := NewFailover(configurations, 1)
			// Rotation starts at the first named profile, even if it is already active.
			for i, want := range tc.sequence {
				rotated, name, err := failover.OnFail()
				if err != nil {
					t.Fatalf("OnFail() call %d: %v", i+1, err)
				}
				if wantName := tc.names[want-1]; !rotated || name != wantName {
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

func TestFailoverRotatesAtThreshold(t *testing.T) {
	for _, threshold := range []int{2, 3, 5} {
		t.Run(fmt.Sprintf("threshold %d", threshold), func(t *testing.T) {
			configurations := failoverTestConfigurations(t, "a", "b")
			if err := configurations.Activate("b"); err != nil {
				t.Fatal(err)
			}
			failover := NewFailover(configurations, threshold)
			previousID := 2
			for _, nextID := range []int{1, 2, 1} {
				for failure := 1; failure <= threshold; failure++ {
					rotated, name, err := failover.OnFail()
					wantRotation := failure == threshold
					wantID, wantName := previousID, ""
					if wantRotation {
						wantID = nextID
						wantName = []string{"a", "b"}[nextID-1]
					}
					if err != nil || rotated != wantRotation || name != wantName {
						t.Fatalf("failure %d: OnFail() = %v, %q, %v; want %v, %q, nil", failure, rotated, name, err, wantRotation, wantName)
					}
					active, err := configurations.Active()
					if err != nil {
						t.Fatal(err)
					}
					if active.ClientID != wantID {
						t.Fatalf("failure %d: active ClientID = %d, want %d", failure, active.ClientID, wantID)
					}
				}
				previousID = nextID
			}
		})
	}
}

func TestFailoverResetPreservesRotationPosition(t *testing.T) {
	failover := NewFailover(failoverTestConfigurations(t, "a", "b"), 3)
	for _, candidate := range []string{"a", "b"} {
		for failure := 1; failure <= 3; failure++ {
			rotated, name, err := failover.OnFail()
			wantName := ""
			if failure == 3 {
				wantName = candidate
			}
			if err != nil || rotated != (failure == 3) || name != wantName {
				t.Fatalf("failure %d: OnFail() = %v, %q, %v; want %q", failure, rotated, name, err, wantName)
			}
		}
		for range 2 {
			if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
				t.Fatalf("OnFail() before reset = %v, %q, %v", rotated, name, err)
			}
		}
		failover.Reset()
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
			failover := NewFailover(configurations, 3)
			for range 2 {
				if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
					t.Fatalf("OnFail() below threshold = %v, %q, %v", rotated, name, err)
				}
			}
			rotated, name, err := failover.OnFail()
			if rotated || name != "" || !errors.Is(err, ErrNoFailoverCandidates) {
				t.Fatalf("OnFail() = %q, %v; want empty name and ErrNoFailoverCandidates", name, err)
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
	configurations := NewConfigurations(directory)
	failover := NewFailover(configurations, 3)
	for range 2 {
		if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
			t.Fatalf("OnFail() below threshold accessed storage: %v, %q, %v", rotated, name, err)
		}
	}
	rotated, name, err := failover.OnFail()
	var pathError *os.PathError
	if rotated || name != "" || !errors.As(err, &pathError) {
		t.Fatalf("OnFail() = %q, %v; want empty name and directory read error", name, err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		writeConfiguration(t, configurations.activePath+"."+name, validTestConfiguration())
	}
	rotated, name, err = failover.OnFail()
	if err != nil || !rotated || name != "a" {
		t.Fatalf("OnFail() after restoring storage = %v, %q, %v; want immediate activation of a", rotated, name, err)
	}
}

func TestFailoverRetriesAfterCandidatesAdded(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a")
	failover := NewFailover(configurations, 3)
	for range 2 {
		if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
			t.Fatalf("OnFail() below threshold = %v, %q, %v", rotated, name, err)
		}
	}
	if rotated, name, err := failover.OnFail(); !errors.Is(err, ErrNoFailoverCandidates) || rotated || name != "" {
		t.Fatalf("OnFail() at threshold = %v, %q, %v; want ErrNoFailoverCandidates", rotated, name, err)
	}
	writeConfiguration(t, configurations.activePath+".b", validTestConfiguration())
	rotated, name, err := failover.OnFail()
	if err != nil || !rotated || name != "a" {
		t.Fatalf("OnFail() after adding candidate = %v, %q, %v; want immediate activation of a", rotated, name, err)
	}
}

func TestFailoverAfterNamedConfigurationDeleted(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a", "b", "c")
	failover := NewFailover(configurations, 1)
	for range 2 {
		if rotated, _, err := failover.OnFail(); err != nil || !rotated {
			t.Fatal(err)
		}
	}
	if err := configurations.Delete("a"); err != nil {
		t.Fatal(err)
	}
	rotated, name, err := failover.OnFail()
	if err != nil {
		t.Fatal(err)
	}
	if !rotated || name != "b" {
		t.Fatalf("OnFail() name = %q, want b", name)
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
	failover := NewFailover(configurations, 3)
	for range 2 {
		if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
			t.Fatalf("OnFail() below threshold = %v, %q, %v", rotated, name, err)
		}
	}
	var syntaxError *json.SyntaxError
	rotated, name, err := failover.OnFail()
	if rotated || name != "a" || !errors.As(err, &syntaxError) {
		t.Fatalf("OnFail() = %q, %v; want a and JSON syntax error", name, err)
	}
	rotated, name, err = failover.OnFail()
	if err != nil {
		t.Fatalf("OnFail() after invalid profile: %v", err)
	}
	if !rotated || name != "b" {
		t.Fatalf("OnFail() name after invalid profile = %q, want b", name)
	}
	active, err := configurations.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.ClientID != 2 {
		t.Fatalf("active ClientID = %d, want 2", active.ClientID)
	}
	if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
		t.Fatalf("OnFail() after successful activation = %v, %q, %v; want a fresh threshold", rotated, name, err)
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
