//go:build windows

package route

type Contract interface {
	Delete(destinationIP string) error
	Print(destinationIP string) ([]byte, error)
	DefaultRoute() (gateway string, ifName string, metric int, err error)
	// BestRoute returns next-hop and interface actually used to reach 'dest'.
	// Gateway == "" or "0.0.0.0"/"::" means on-link.
	BestRoute(dest string) (gateway string, ifName string, ifIndex int, metric int, err error)
}
