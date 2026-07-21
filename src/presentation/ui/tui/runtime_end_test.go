package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveRuntimeEnd(t *testing.T) {
	uiErr := errors.New("ui failed")
	workerErr := errors.New("worker failed")

	tests := []struct {
		name       string
		uiResult   RuntimeUIResult
		workerErr  error
		isUserExit func(error) bool
		want       error
		wantText   string
		wantLogged bool
		nilLogger  bool
		wantNil    bool
	}{
		{
			name:       "worker error takes precedence over UI error",
			uiResult:   RuntimeUIResult{Err: uiErr},
			workerErr:  workerErr,
			want:       workerErr,
			wantLogged: true,
		},
		{
			name:       "UI error after clean worker",
			uiResult:   RuntimeUIResult{Err: uiErr},
			wantText:   "runtime UI failed: ui failed",
			wantLogged: true,
		},
		{
			name:     "user exit is clean",
			uiResult: RuntimeUIResult{Err: errors.New("user exit")},
			isUserExit: func(err error) bool {
				return err != nil && err.Error() == "user exit"
			},
			wantNil: true,
		},
		{
			name:      "worker error takes precedence over user exit",
			uiResult:  RuntimeUIResult{Err: errors.New("user exit")},
			workerErr: workerErr,
			isUserExit: func(err error) bool {
				return err != nil && err.Error() == "user exit"
			},
			want: workerErr,
		},
		{
			name:     "user quit requests reconfiguration",
			uiResult: RuntimeUIResult{UserQuit: true},
			want:     errReconfigureRequested,
		},
		{
			name:      "worker error takes precedence over reconfiguration",
			uiResult:  RuntimeUIResult{UserQuit: true},
			workerErr: workerErr,
			want:      workerErr,
		},
		{
			name:      "normal UI result returns worker result",
			uiResult:  RuntimeUIResult{},
			workerErr: workerErr,
			want:      workerErr,
		},
		{
			name:      "nil callbacks are allowed",
			uiResult:  RuntimeUIResult{Err: uiErr},
			wantText:  "runtime UI failed: ui failed",
			nilLogger: true,
		},
		{
			name:     "normal clean result",
			uiResult: RuntimeUIResult{},
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logged := false
			var onRuntimeUIError func(error)
			if !tt.nilLogger {
				onRuntimeUIError = func(error) { logged = true }
			}

			err := resolveRuntimeEnd(
				tt.uiResult,
				tt.workerErr,
				tt.isUserExit,
				onRuntimeUIError,
			)
			if tt.wantNil && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
			if tt.wantText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantText)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantText, err)
			}
			if logged != tt.wantLogged {
				t.Fatalf("expected logged=%v, got %v", tt.wantLogged, logged)
			}
		})
	}
}
