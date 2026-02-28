package capabilities

import "testing"

func TestServerSupported_Windows(t *testing.T) {
	if ServerSupported() {
		t.Fatal("expected ServerSupported() == false on windows")
	}
}
