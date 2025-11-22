package args

import (
	"os"
	"reflect"
	"testing"
)

func TestDefaultProviderArgs(t *testing.T) {
	original := os.Args
	defer func() { os.Args = original }()

	tests := []struct {
		name     string
		osArgs   []string
		expected []string
	}{
		{
			name:     "binary with two arguments",
			osArgs:   []string{"tungo", "--config=/etc/tungo/foo.json", "extra"},
			expected: []string{"--config=/etc/tungo/foo.json", "extra"},
		},
		{
			name:     "only binary name",
			osArgs:   []string{"tungo"},
			expected: []string{},
		},
	}

	p := NewDefaultProvider()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.osArgs

			got := p.Args()
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("Args() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}
