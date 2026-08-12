package settings

import "time"

type DialTimeoutMs int

func (d DialTimeoutMs) Int() int {
	return int(d)
}

func (d DialTimeoutMs) Duration() time.Duration {
	return time.Duration(d) * time.Millisecond
}
