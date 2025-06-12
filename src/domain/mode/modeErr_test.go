package mode

import "testing"

func TestErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{NewNoModeProvided(), "no mode provided"},
		{NewInvalidModeProvided("prod"), "prod is not a valid mode"},
		{NewInvalidModeProvided(""), "empty string is not a valid mode"},
		{NewInvalidExecPathProvided(), "missing execution binary path as first argument"},
	}

	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Fatalf("want %q, got %q", c.want, got)
		}
	}
}
