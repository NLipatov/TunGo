package client

import (
	"errors"
	"fmt"
	"testing"
	"time"

	clientconfig "tungo/internal/config/client"
)

func TestAttemptFailoverCountsSessionErrors(t *testing.T) {
	sessionErr := errors.New("connection failed")
	for _, tc := range []struct {
		name           string
		sessionReadyAt time.Time
		sessionErr     error
		count          int
		want           int
	}{
		{name: "first connection failure", sessionErr: sessionErr, want: 1},
		{name: "not yet ready", sessionErr: sessionErr, count: 3, want: 4},
		{name: "short session", sessionReadyAt: time.Now().Add(-time.Minute), sessionErr: sessionErr, count: 3, want: 4},
		{name: "stable session", sessionReadyAt: time.Now().Add(-6 * time.Minute), sessionErr: sessionErr, count: 4, want: 1},
		{name: "no error", count: 4, want: 4},
		{name: "resume", sessionErr: ErrResumeDetected, count: 4, want: 4},
		{name: "wrapped resume", sessionErr: fmt.Errorf("session: %w", ErrResumeDetected), count: 4, want: 4},
		{name: "resume after stable session", sessionReadyAt: time.Now().Add(-6 * time.Minute), sessionErr: ErrResumeDetected, count: 4, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &Client{
				failoverThreshold: 5,
				failoverCount:     tc.count,
				sessionReadyAt:    tc.sessionReadyAt,
			}
			if err := client.attemptFailover(tc.sessionErr); err != nil {
				t.Fatalf("attemptFailover() error = %v", err)
			}
			if client.failoverCount != tc.want {
				t.Fatalf("failoverCount = %d, want %d", client.failoverCount, tc.want)
			}
		})
	}
}

func TestAttemptFailoverRotatesAtThreshold(t *testing.T) {
	for _, threshold := range []int{2, 5} {
		t.Run(fmt.Sprintf("threshold %d", threshold), func(t *testing.T) {
			client := newTestClient(t)
			client.failoverThreshold = threshold
			configuration := clientTestConfiguration()
			importClientConfiguration(t, client.configurations, "a", configuration)
			configuration.ClientID = 2
			importClientConfiguration(t, client.configurations, "b", configuration)
			if err := client.configurations.Activate("b"); err != nil {
				t.Fatal(err)
			}

			previousID := 2
			for _, nextID := range []int{1, 2, 1} {
				for failure := 1; failure <= threshold; failure++ {
					if err := client.attemptFailover(errors.New("connection failed")); err != nil {
						t.Fatal(err)
					}
					wantID, wantCount := previousID, failure
					if failure == threshold {
						wantID, wantCount = nextID, 0
					}
					active, err := client.configurations.Active()
					if err != nil {
						t.Fatal(err)
					}
					if active.ClientID != wantID || client.failoverCount != wantCount {
						t.Fatalf("failure %d: active ClientID = %d, count = %d; want %d, %d",
							failure, active.ClientID, client.failoverCount, wantID, wantCount)
					}
				}
				previousID = nextID
			}
		})
	}
}

func TestAttemptFailoverRetriesAfterFailure(t *testing.T) {
	client := newTestClient(t)
	configuration := clientTestConfiguration()
	importClientConfiguration(t, client.configurations, "a", configuration)
	client.failoverCount = client.failoverThreshold - 1
	sessionErr := errors.New("connection failed")
	if err := client.attemptFailover(sessionErr); !errors.Is(err, clientconfig.ErrNoFailoverCandidates) {
		t.Fatalf("attemptFailover() error = %v, want ErrNoFailoverCandidates", err)
	}
	if client.failoverCount != client.failoverThreshold {
		t.Fatalf("failoverCount = %d, want %d after failed failover", client.failoverCount, client.failoverThreshold)
	}

	configuration.ClientID = 2
	importClientConfiguration(t, client.configurations, "b", configuration)
	if err := client.attemptFailover(sessionErr); err != nil {
		t.Fatalf("attemptFailover() after adding a candidate: %v", err)
	}
	active, err := client.configurations.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.ClientID != 1 || client.failoverCount != 0 {
		t.Fatalf("active ClientID = %d, count = %d; want 1, 0", active.ClientID, client.failoverCount)
	}
}
