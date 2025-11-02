//go:build windows

package ipconfig

import (
	"fmt"
	"golang.org/x/sys/windows"
)

// Wrapper implements Contract using native Windows APIs (no shell commands).
type Wrapper struct{}

func NewWrapper() Contract {
	return &Wrapper{}
}

// FlushDNS clears the system resolver cache via DnsFlushResolverCache (dnsapi.dll).
func (w *Wrapper) FlushDNS() error {
	dnsApi := windows.NewLazySystemDLL("dnsapi.dll")
	proc := dnsApi.NewProc("DnsFlushResolverCache")
	if err := dnsApi.Load(); err != nil {
		return fmt.Errorf("failed to load dnsapi.dll: %w", err)
	}
	r, _, callErr := proc.Call()
	if r == 0 {
		return fmt.Errorf("DnsFlushResolverCache failed: %v", callErr)
	}
	return nil
}
