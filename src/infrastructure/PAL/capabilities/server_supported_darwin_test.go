package capabilities

import "testing"

func TestServerSupported_Darwin(t *testing.T) {
	if ServerSupported() {
		t.Fatal("expected ServerSupported() == false on darwin")
	}
}
