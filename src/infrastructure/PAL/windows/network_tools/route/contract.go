//go:build windows

package route

type Contract interface {
	Delete(destinationIP string) error
	Print(destinationIP string) ([]byte, error)
	DefaultRoute() (gateway string, ifName string, metric int, err error)
}
