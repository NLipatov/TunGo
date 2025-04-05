package mode_selection

import (
	"errors"
	"os"
	"testing"
	"tungo/domain/mode"
)

func TestPromptAppMode_Mode(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		// simulatedInput is used when prompt is expected (i.e. len(arguments) < 2)
		simulatedInput  string
		wantMode        mode.Mode
		wantErr         bool
		expectedErrMsg  string
		expectedErrType error
	}{
		{
			name:            "empty arguments slice",
			arguments:       []string{},
			wantMode:        mode.Unknown,
			wantErr:         true,
			expectedErrMsg:  "missing execution binary path as first argument",
			expectedErrType: mode.NewInvalidExecPathProvided(),
		},
		{
			name:            "no mode provided, prompt returns empty string",
			arguments:       []string{"program"},
			simulatedInput:  "\n", // empty input from prompt
			wantMode:        mode.Unknown,
			wantErr:         true,
			expectedErrMsg:  "empty string is not a valid mode",
			expectedErrType: mode.NewInvalidModeProvided(""),
		},
		{
			name:           "prompt returns client mode",
			arguments:      []string{"program"},
			simulatedInput: "c\n",
			wantMode:       mode.Client,
			wantErr:        false,
		},
		{
			name:           "prompt returns server mode",
			arguments:      []string{"program"},
			simulatedInput: "s\n",
			wantMode:       mode.Server,
			wantErr:        false,
		},
		{
			name:      "arguments provided already (client)",
			arguments: []string{"program", "c"},
			wantMode:  mode.Client,
			wantErr:   false,
		},
		{
			name:            "arguments provided already (invalid)",
			arguments:       []string{"program", "x"},
			wantMode:        mode.Unknown,
			wantErr:         true,
			expectedErrMsg:  "x is not a valid mode",
			expectedErrType: mode.NewInvalidModeProvided("x"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If simulated input is needed, override os.Stdin using a pipe.
			if tt.simulatedInput != "" {
				origStdin := os.Stdin
				pr, pw, err := os.Pipe()
				if err != nil {
					t.Fatalf("failed to create pipe: %v", err)
				}
				// Write the simulated input and close the writer.
				_, err = pw.WriteString(tt.simulatedInput)
				if err != nil {
					t.Fatalf("failed to write simulated input: %v", err)
				}
				_ = pw.Close()
				os.Stdin = pr
				defer func() {
					os.Stdin = origStdin
				}()
			}

			p := NewPromptAppMode(tt.arguments)
			gotMode, err := p.Mode()

			if gotMode != tt.wantMode {
				t.Errorf("Mode() gotMode = %v, want %v", gotMode, tt.wantMode)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Mode() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				if tt.expectedErrMsg != "" && err.Error() != tt.expectedErrMsg {
					t.Errorf("Mode() error message = %q, want %q", err.Error(), tt.expectedErrMsg)
				}

				if tt.expectedErrType != nil {
					var noModeProvided mode.NoModeProvided
					var invalidModeProvided mode.InvalidModeProvided
					var invalidExecPathProvided mode.InvalidExecPathProvided
					switch {
					case errors.As(tt.expectedErrType, &noModeProvided):
						var target mode.NoModeProvided
						if !errors.As(err, &target) {
							t.Errorf("expected error type %T, got %T", tt.expectedErrType, err)
						}
					case errors.As(tt.expectedErrType, &invalidModeProvided):
						var target mode.InvalidModeProvided
						if !errors.As(err, &target) {
							t.Errorf("expected error type %T, got %T", tt.expectedErrType, err)
						}
					case errors.As(tt.expectedErrType, &invalidExecPathProvided):
						var target mode.InvalidExecPathProvided
						if !errors.As(err, &target) {
							t.Errorf("expected error type %T, got %T", tt.expectedErrType, err)
						}
					}
				}
			}
		})
	}
}
