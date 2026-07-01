package command

import "testing"

func TestErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrNoCommandProvided, "no command provided"},
		{InvalidCommand("prod"), "prod is not a valid command"},
		{InvalidCommand(""), "empty string is not a valid command"},
		{ErrInvalidExecPathProvided, "missing execution binary path as first argument"},
	}

	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Fatalf("want %q, got %q", c.want, got)
		}
	}
}
