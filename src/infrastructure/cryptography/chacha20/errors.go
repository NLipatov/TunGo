package chacha20

import "fmt"

var (
	ErrEpochExhausted = fmt.Errorf("epoch exhausted; requires full re-handshake")
)
