package presentation

import (
	"errors"
	"testing"
	"tungo/domain/mode"
)

func TestAppMode_Mode(t *testing.T) {
	tests := []struct {
		name            string
		arguments       []string
		wantMode        mode.Mode
		wantErr         bool
		expectedErrMsg  string
		expectedErrType interface{}
	}{
		{
			name:            "no arguments provided",
			arguments:       []string{"program"},
			wantMode:        mode.Unknown,
			wantErr:         true,
			expectedErrMsg:  "no mode provided",
			expectedErrType: mode.NewNoModeProvided(),
		},
		{
			name:      "client mode ('c')",
			arguments: []string{"program", "c"},
			wantMode:  mode.Client,
			wantErr:   false,
		},
		{
			name:      "server mode ('s')",
			arguments: []string{"program", "s"},
			wantMode:  mode.Server,
			wantErr:   false,
		},
		{
			name:            "invalid mode",
			arguments:       []string{"program", "x"},
			wantMode:        mode.Unknown,
			wantErr:         true,
			expectedErrMsg:  "x is not a valid mode",
			expectedErrType: mode.NewInvalidModeProvided("x"),
		},
		{
			name:      "client mode with extra spaces and mixed case",
			arguments: []string{"program", " C "},
			wantMode:  mode.Client,
			wantErr:   false,
		},
		{
			name:      "server mode in uppercase",
			arguments: []string{"program", "S"},
			wantMode:  mode.Server,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appMode := NewAppMode(tt.arguments)
			gotMode, err := appMode.Mode()

			if gotMode != tt.wantMode {
				t.Errorf("Mode() gotMode = %v, want %v", gotMode, tt.wantMode)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Mode() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				// Check error message
				if tt.expectedErrMsg != "" && err.Error() != tt.expectedErrMsg {
					t.Errorf("Mode() error message = %v, want %v", err.Error(), tt.expectedErrMsg)
				}
				// Check error type if expected type is provided
				if tt.expectedErrType != nil {
					switch tt.expectedErrType.(type) {
					case mode.NoModeProvided:
						var target mode.NoModeProvided
						if !errors.As(err, &target) {
							t.Errorf("expected error type %T, got %T", tt.expectedErrType, err)
						}
					case mode.InvalidModeProvided:
						var target mode.InvalidModeProvided
						if !errors.As(err, &target) {
							t.Errorf("expected error type %T, got %T", tt.expectedErrType, err)
						}
					}
				}
			}
		})
	}
}
