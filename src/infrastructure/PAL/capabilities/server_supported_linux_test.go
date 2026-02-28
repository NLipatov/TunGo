package capabilities

import "testing"

func TestServerSupported_Linux(t *testing.T) {
	if !ServerSupported() {
		t.Fatal("expected ServerSupported() == true on linux")
	}
}
