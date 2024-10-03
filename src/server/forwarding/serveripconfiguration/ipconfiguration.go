package serveripconfiguration

import (
	"etha-tunnel/network"
	"etha-tunnel/network/ip"
	"etha-tunnel/network/iptables"
	"fmt"
	"log"
	"os"
)

func Configure(tunFile *os.File) error {
	externalIfName, err := ip.RouteDefault()
	if err != nil {
		return err
	}

	err = iptables.EnableMasquerade(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	err = setupForwarding(tunFile, externalIfName)
	if err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}

	if err != nil {
		return err
	}

	log.Printf("server configured\n")
	return nil
}

func Unconfigure(tunFile *os.File) {
	tunName, err := network.GetIfName(tunFile)
	if err != nil {
		log.Printf("failed to determing tunnel ifName: %s\n", err)
	}

	err = iptables.DisableMasquerade(tunName)
	if err != nil {
		log.Printf("failed to disbale NAT: %s\n", err)
	}

	err = clearForwarding(tunFile, tunName)
	if err != nil {
		log.Printf("failed to disbale forwarding: %s\n", err)
	}

	log.Printf("server unconfigured\n")
}

func setupForwarding(tunFile *os.File, extIface string) error {
	// Get the name of the TUN interface
	tunName, err := network.GetIfName(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	// Set up iptables rules
	err = iptables.AcceptForwardFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	err = iptables.AcceptForwardFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	return nil
}

func clearForwarding(tunFile *os.File, extIface string) error {
	tunName, err := network.GetIfName(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	err = iptables.DropForwardFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}

	err = iptables.DropForwardFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}
	return nil
}
