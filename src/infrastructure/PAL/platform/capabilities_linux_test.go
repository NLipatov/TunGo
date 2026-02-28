package platform

import "testing"

func TestServerModeSupported_Linux(t *testing.T) {
	if !Capabilities().ServerModeSupported() {
		t.Fatal("expected ServerModeSupported() == true on linux")
	}
}
