package mem

import "testing"

func TestZeroBytes_ZeroesNonEmptySlice(t *testing.T) {
	buf := []byte{1, 2, 3, 4, 5}
	ZeroBytes(buf)
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("expected buf[%d] to be zero, got %d", i, b)
		}
	}
}

func TestZeroBytes_EmptyAndNilSlices(t *testing.T) {
	empty := []byte{}
	ZeroBytes(empty)

	var nilSlice []byte
	ZeroBytes(nilSlice)
}
