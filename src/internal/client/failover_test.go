package client

import (
	"errors"
	"fmt"
	"testing"
	"time"
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
