package runtime

import "testing"

func TestNewRejectsInvalidMode(t *testing.T) {
	if _, err := New(0); err == nil {
		t.Fatal("New() accepted an invalid mode")
	}
}
