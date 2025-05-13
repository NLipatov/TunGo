package ip

//
//import (
//	"fmt"
//	"tungo/infrastructure/PAL/linux/sysctl"
//)
//
//func UpNewTun(ifName string) (string, error) {
//	err := enableIPv4Forwarding()
//	if err != nil {
//		return "", err
//	}
//
//	_, err = LinkAdd(ifName)
//	if err != nil {
//		return "", err
//	}
//
//	_, err = LinkSetUp(ifName)
//	if err != nil {
//		return "", err
//	}
//
//	return ifName, nil
//}
//
//func enableIPv4Forwarding() error {
//	// ToDo: ipv6 forwarding
//	output, err := sysctl.NetIpv4IpForward()
//	if err != nil {
//		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
//	}
//
//	if string(output) == "net.ipv4.ip_forward = 1\n" {
//		return nil
//	}
//
//	output, err = sysctl.WNetIpv4IpForward()
//	if err != nil {
//		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
//	}
//	return nil
//}
