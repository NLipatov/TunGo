package client

import (
	"errors"
	"testing"
)

// stubResolver implements Resolver for testing.
type stubResolver struct {
	path   string
	err    error
	called bool
}

func (s *stubResolver) Resolve() (string, error) {
	s.called = true
	return s.path, s.err
}

// fakeArgsProvider implements args.Provider for testing.
type fakeArgsProvider struct {
	args []string
}

func (f fakeArgsProvider) Args() []string {
	return f.args
}

func TestArgumentResolver_Resolve(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		basePath       string
		baseErr        error
		wantPath       string
		wantErr        bool
		wantBaseCalled bool
	}{
		{
			name:           "no config flag -> fallback to base resolver",
			args:           []string{},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/etc/tungo/client_configuration.json",
			wantBaseCalled: true,
		},
		{
			name:           "--config=/path/to/file",
			args:           []string{"--config=/custom/config.json"},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/custom/config.json",
			wantBaseCalled: false,
		},
		{
			name:           "--config= with empty value -> fallback",
			args:           []string{"--config="},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/etc/tungo/client_configuration.json",
			wantBaseCalled: true,
		},
		{
			name:           "--config /path/to/file",
			args:           []string{"--config", "/custom/config.json"},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/custom/config.json",
			wantBaseCalled: false,
		},
		{
			name:           "--config with empty next token -> fallback",
			args:           []string{"--config", ""},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/etc/tungo/client_configuration.json",
			wantBaseCalled: true,
		},
		{
			name:           "--config followed by another flag -> fallback",
			args:           []string{"--config", "-v"},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/etc/tungo/client_configuration.json",
			wantBaseCalled: true,
		},
		{
			name:           "--config without value at end -> fallback",
			args:           []string{"--config"},
			basePath:       "/etc/tungo/client_configuration.json",
			wantPath:       "/etc/tungo/client_configuration.json",
			wantBaseCalled: true,
		},
		{
			name:           "base resolver returns error and no config flag",
			args:           []string{},
			baseErr:        errors.New("boom"),
			wantErr:        true,
			wantBaseCalled: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			// arrange
			base := &stubResolver{
				path: tt.basePath,
				err:  tt.baseErr,
			}
			provider := fakeArgsProvider{args: tt.args}
			resolver := NewArgumentResolver(base, provider)

			// act
			gotPath, err := resolver.Resolve()

			// assert
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if gotPath != tt.wantPath {
				t.Fatalf("Resolve() path = %q, want %q", gotPath, tt.wantPath)
			}

			if base.called != tt.wantBaseCalled {
				t.Fatalf("base resolver called = %v, want %v", base.called, tt.wantBaseCalled)
			}
		})
	}
}
