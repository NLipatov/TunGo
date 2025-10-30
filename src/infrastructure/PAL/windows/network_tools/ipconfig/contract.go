//go:build windows

package ipconfig

type Contract interface {
	FlushDNS() error
}
