//go:build windows

package ipcfg

const (
	ipcfgMetric = 1
	// v4SplitOne covers half of IPv4 address space
	// (addresses between 0.0.0.0 and 127.255.255.255)
	v4SplitOne = "0.0.0.0/1"
	// v4SplitTwo v4SplitOne covers half of IPv4 address space
	// (addresses between 128.0.0.0 and 255.255.255.255)
	v4SplitTwo = "128.0.0.0/1"
	// v6SplitOne covers addresses between :: (0000:0000:0000:0000:0000:0000:0000:0000) and 7fff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitOne = "::/1"
	// v6SplitTwo covers addresses between 8000:: (8000:0000:0000:0000:0000:0000:0000:0000) and ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitTwo = "8000::/1"
)
