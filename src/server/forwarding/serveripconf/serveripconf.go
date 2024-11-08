package serveripconf

import (
	"etha-tunnel/network/ip"
	"etha-tunnel/settings"
	"fmt"
	"log"
	"os"
)

func SetupServerTun(settings settings.ConnectionSettings) (*os.File, error) {
	_, _ = ip.LinkDel(settings.InterfaceName)

	name, err := ip.UpNewTun(settings.InterfaceName)
	if err != nil {
		log.Fatalf("failed to create interface %v: %v", settings.InterfaceName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	serverIp, err := ip.AllocateServerIp(settings.InterfaceIPCIDR)
	if err != nil {
		log.Fatalf("failed to allocate server ip %v: %v", settings.InterfaceName, err)
	}

	cidrServerIp, err := ip.ToCIDR(settings.InterfaceIPCIDR, serverIp)
	if err != nil {
		log.Fatalf("failed to conver server ip to CIDR format: %s", err)
	}
	_, err = ip.LinkAddrAdd(settings.InterfaceName, cidrServerIp)
	if err != nil {
		log.Fatalf("failed to conver server ip to CIDR format: %s", err)
	}
	fmt.Printf("assigned IP %s to interface %s\n", settings.ConnectionPort, settings.InterfaceName)

	setMtuErr := ip.SetMtu(settings.InterfaceName, settings.MTU)
	if setMtuErr != nil {
		log.Fatalf("failed to set MTU: %s", setMtuErr)
	}

	tunFile, err := ip.OpenTunByName(settings.InterfaceName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}

	return tunFile, nil
}
