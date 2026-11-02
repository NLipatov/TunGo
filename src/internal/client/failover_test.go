package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientconfig "tungo/internal/config/client"
)

func TestAttemptFailoverHandlesSessionOutcome(t *testing.T) {
	sessionErr := errors.New("connection failed")
	for _, tc := range []struct {
		name              string
		readyFor          time.Duration
		sessionErr        error
		previousFailures  int
		remainingFailures int
	}{
		{name: "first connection failure", sessionErr: sessionErr, remainingFailures: 2},
		{name: "not yet ready", sessionErr: sessionErr, previousFailures: 2},
		{name: "short session", readyFor: time.Minute, sessionErr: sessionErr, previousFailures: 2},
		{name: "stable session", readyFor: 6 * time.Minute, sessionErr: sessionErr, previousFailures: 2, remainingFailures: 2},
		{name: "five minute session", readyFor: 5 * time.Minute, sessionErr: sessionErr, previousFailures: 2, remainingFailures: 2},
		{name: "resume", readyFor: time.Minute, sessionErr: ErrResumeDetected, previousFailures: 2, remainingFailures: 1},
		{name: "wrapped resume", sessionErr: fmt.Errorf("session: %w", ErrResumeDetected), previousFailures: 2, remainingFailures: 1},
		{name: "resume after stable session", readyFor: 6 * time.Minute, sessionErr: ErrResumeDetected, previousFailures: 2, remainingFailures: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t)
			configuration := clientTestConfiguration()
			importClientConfiguration(t, client.configurations, "a", configuration)
			configuration.ClientID = 2
			importClientConfiguration(t, client.configurations, "b", configuration)
			if err := client.configurations.Activate("b"); err != nil {
				t.Fatal(err)
			}
			for range tc.previousFailures {
				client.attemptFailover(sessionErr)
			}
			if tc.readyFor > 0 {
				client.sessionReadyAt = time.Now().Add(-tc.readyFor)
			}
			client.attemptFailover(tc.sessionErr)
			// Subsequent connection attempts fail before reaching readiness.
			client.sessionReadyAt = time.Time{}
			for failure := 0; failure <= tc.remainingFailures; failure++ {
				if failure > 0 {
					client.attemptFailover(sessionErr)
				}
				wantID := 2
				if failure == tc.remainingFailures {
					wantID = 1
				}
				active, err := client.configurations.Active()
				if err != nil {
					t.Fatal(err)
				}
				if active.ClientID != wantID {
					t.Fatalf("after %d additional failures: active ClientID = %d, want %d", failure, active.ClientID, wantID)
				}
			}
		})
	}
}

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
			failover := newFailover(configurations, 1)
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
			failover := newFailover(configurations, threshold)
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
	failover := newFailover(failoverTestConfigurations(t, "a", "b"), 3)
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
			initial := clientTestConfiguration()
			initial.ClientID = 7
			importClientConfiguration(t, configurations, "initial", initial)
			if err := configurations.Activate("initial"); err != nil {
				t.Fatal(err)
			}
			if err := configurations.Delete("initial"); err != nil {
				t.Fatal(err)
			}
			failover := newFailover(configurations, 3)
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
	configurations := clientconfig.NewConfigurations(directory)
	failover := newFailover(configurations, 3)
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
		importClientConfiguration(t, configurations, name, clientTestConfiguration())
	}
	rotated, name, err = failover.OnFail()
	if err != nil || !rotated || name != "a" {
		t.Fatalf("OnFail() after restoring storage = %v, %q, %v; want immediate activation of a", rotated, name, err)
	}
}

func TestFailoverRetriesAfterCandidatesAdded(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a")
	failover := newFailover(configurations, 3)
	for range 2 {
		if rotated, name, err := failover.OnFail(); err != nil || rotated || name != "" {
			t.Fatalf("OnFail() below threshold = %v, %q, %v", rotated, name, err)
		}
	}
	if rotated, name, err := failover.OnFail(); !errors.Is(err, ErrNoFailoverCandidates) || rotated || name != "" {
		t.Fatalf("OnFail() at threshold = %v, %q, %v; want ErrNoFailoverCandidates", rotated, name, err)
	}
	importClientConfiguration(t, configurations, "b", clientTestConfiguration())
	rotated, name, err := failover.OnFail()
	if err != nil || !rotated || name != "a" {
		t.Fatalf("OnFail() after adding candidate = %v, %q, %v; want immediate activation of a", rotated, name, err)
	}
}

func TestFailoverAfterNamedConfigurationDeleted(t *testing.T) {
	configurations := failoverTestConfigurations(t, "a", "b", "c")
	failover := newFailover(configurations, 1)
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
	directory := t.TempDir()
	configurations := clientconfig.NewConfigurations(directory)
	for i, name := range []string{"a", "b"} {
		configuration := clientTestConfiguration()
		configuration.ClientID = i + 1
		importClientConfiguration(t, configurations, name, configuration)
	}
	// Bypass Import validation to simulate a named file damaged externally.
	if err := os.WriteFile(filepath.Join(directory, "client_configuration.json.a"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	failover := newFailover(configurations, 3)
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

func failoverTestConfigurations(t *testing.T, names ...string) *clientconfig.Configurations {
	t.Helper()
	configurations := clientconfig.NewConfigurations(t.TempDir())
	for i, name := range names {
		configuration := clientTestConfiguration()
		configuration.ClientID = i + 1
		importClientConfiguration(t, configurations, name, configuration)
	}
	return configurations
}
