package network

import (
	"etha-tunnel/network/ip"
	"etha-tunnel/settings/server"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"os/exec"
	"strings"
	"unsafe"
)

const (
	IFNAMSIZ  = 16         // Max if name size, bytes
	TUNSETIFF = 0x400454ca // Code to create TUN/TAP if via ioctl
	IFF_TUN   = 0x0001     // Enabling TUN flag
	IFF_NO_PI = 0x1000     // Disabling PI (Packet Information)
)

func UpNewTun(ifName string) (string, error) {
	err := enableIPv4Forwarding()
	if err != nil {
		return "", err
	}

	_, err = ip.LinkAdd(ifName)
	if err != nil {
		return "", err
	}

	_, err = ip.LinkSetUp(ifName)
	if err != nil {
		return "", err
	}

	return ifName, nil
}

func OpenTunByName(ifname string) (*os.File, error) {
	tunFilePath := "/dev/net/tun"
	tun, err := os.OpenFile(tunFilePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %v: %v", tunFilePath, err)
	}

	var req IfReq
	copy(req.Name[:], ifname)
	req.Flags = IFF_TUN | IFF_NO_PI

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, tun.Fd(), uintptr(TUNSETIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		tun.Close()
		return nil, fmt.Errorf("ioctl failed: %v", errno)
	}

	return tun, nil
}

func enableIPv4Forwarding() error {
	// ToDo: ipv6 forwarding
	cmd := exec.Command("sysctl", "net.ipv4.ip_forward")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
	}

	if string(output) == "net.ipv4.ip_forward = 1\n" {
		return nil
	}

	cmd = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
	}
	return nil
}

func CreateNewTun(conf *server.Conf) error {
	_, _ = ip.LinkDel(conf.IfName)

	name, err := UpNewTun(conf.IfName)
	if err != nil {
		log.Fatalf("failed to create interface %v: %v", conf.IfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	_, err = ip.LinkAddrAdd(conf.IfName, conf.IfIP)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", conf.TCPPort, conf.IfName)

	return nil
}

func GetIfName(tunFile *os.File) (string, error) {
	var ifr IfReq

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		tunFile.Fd(),
		uintptr(unix.TUNGETIFF),
		uintptr(unsafe.Pointer(&ifr)),
	)
	if errno != 0 {
		return "", errno
	}

	ifName := string(ifr.Name[:])
	ifName = strings.Trim(string(ifr.Name[:]), "\x00")
	return ifName, nil
}
