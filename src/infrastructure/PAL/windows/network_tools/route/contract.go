//go:build windows

package route

type Contract interface {
	Delete(destinationIP string) error
	Print(destinationIP string) ([]byte, error)
	DefaultRoute() (gw, ifName string, metric int, err error)
	BestRoute(dest string) (string, string, int, int, error)
}
