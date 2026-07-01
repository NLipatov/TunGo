package command_selection

import (
	"errors"
	"testing"
	"tungo/domain/command"
)

func TestArgsAppCommand_Command(t *testing.T) {
	tests := []struct {
		name           string
		arguments      []string
		wantCommand    command.Command
		wantErr        bool
		expectedErrMsg string
		expectedErr    error
	}{
		{
			name:           "empty arguments slice",
			arguments:      []string{},
			wantCommand:    command.Unknown,
			wantErr:        true,
			expectedErrMsg: "missing execution binary path as first argument",
			expectedErr:    command.ErrInvalidExecPathProvided,
		},
		{
			name:           "no command provided",
			arguments:      []string{"program"},
			wantCommand:    command.Unknown,
			wantErr:        true,
			expectedErrMsg: "no command provided",
			expectedErr:    command.ErrNoCommandProvided,
		},
		{
			name:        "start client command ('c')",
			arguments:   []string{"program", "c"},
			wantCommand: command.StartClient,
			wantErr:     false,
		},
		{
			name:        "start server command ('s')",
			arguments:   []string{"program", "s"},
			wantCommand: command.StartServer,
			wantErr:     false,
		},
		{
			name:           "invalid command",
			arguments:      []string{"program", "x"},
			wantCommand:    command.Unknown,
			wantErr:        true,
			expectedErrMsg: "x is not a valid command",
			expectedErr:    command.InvalidCommand("x"),
		},
		{
			name:        "client command with extra spaces and mixed case",
			arguments:   []string{"program", " C "},
			wantCommand: command.StartClient,
			wantErr:     false,
		},
		{
			name:        "server command in uppercase",
			arguments:   []string{"program", "S"},
			wantCommand: command.StartServer,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appCommand := NewArgsAppCommand(tt.arguments)
			gotCommand, err := appCommand.Command()

			if gotCommand != tt.wantCommand {
				t.Errorf("Command() gotCommand = %v, want %v", gotCommand, tt.wantCommand)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Command() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				// Check error message
				if tt.expectedErrMsg != "" && err.Error() != tt.expectedErrMsg {
					t.Errorf("Command() error message = %q, want %q", err.Error(), tt.expectedErrMsg)
				}
				if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("Command() error = %v, want %v", err, tt.expectedErr)
				}
			}
		})
	}
}
