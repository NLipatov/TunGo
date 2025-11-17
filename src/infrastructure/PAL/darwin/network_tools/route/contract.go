//go:build darwin

package route

// Contract defines all the routing operations supported by Wrapper.
type Contract interface {
	// Get determines and installs a route to destIP.
	Get(destIP string) error
	// Add adds a route to ip via the named interface.
	Add(ip, iFace string) error
	// AddViaGateway adds a route to ip via the specified gateway.
	AddViaGateway(ip, gw string) error
	// AddSplit installs the two half-routes via dev.
	AddSplit(dev string) error
	DelSplit(dev string) error
	// Del deletes any route pointing at destIP.
	Del(destIP string) error
	// DefaultGateway queries the system’s default gateway IP.
	DefaultGateway() (string, error)
}
