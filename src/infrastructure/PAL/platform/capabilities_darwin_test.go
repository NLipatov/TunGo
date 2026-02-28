package platform

import "testing"

func TestServerModeSupported_Darwin(t *testing.T) {
	if Capabilities().ServerModeSupported() {
		t.Fatal("expected ServerModeSupported() == false on darwin")
	}
}
