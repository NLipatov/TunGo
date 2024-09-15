package network

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"unsafe"
)

const (
	IFNAMSIZ  = 16         // Max if name size, bytes
	TUNSETIFF = 0x400454ca // Code to create TUN/TAP if via ioctl
	IFF_TUN   = 0x0001     // Enabling TUN flag
	IFF_NO_PI = 0x1000     // Disabling PI (Packet Information)
)

type ifreq struct {
	Name  [IFNAMSIZ]byte
	Flags uint16
	_     [24]byte
}

func UpNewTun(ifName string) (string, error) {
	err := enableIPv4Forwarding()
	if err != nil {
		return "", err
	}

	createTun := exec.Command("ip", "tuntap", "add", "dev", ifName, "mode", "tun")
	createTunOutput, err := createTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create TUN %v: %v, output: %s", ifName, err, createTunOutput)
	}

	startTun := exec.Command("ip", "link", "set", "dev", ifName, "up")
	startTunOutput, err := startTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start TUN %v: %v, output: %s", ifName, err, startTunOutput)
	}

	return ifName, nil
}

func DeleteInterface(ifName string) error {
	cmd := exec.Command("ip", "link", "delete", ifName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}
	return nil
}

func OpenTunByName(ifname string) (*os.File, error) {
	tunFilePath := "/dev/net/tun"
	tun, err := os.OpenFile(tunFilePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %v: %v", tunFilePath, err)
	}

	var req ifreq
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
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
	}
	return nil
}

func ReadFromTun(tun *os.File) ([]byte, error) {
	buf := make([]byte, 65535) // Max IP packet size
	n, err := tun.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read from TUN: %v", err)
	}
	return buf[:n], nil
}

func WriteToTun(tun *os.File, data []byte) error {
	_, err := tun.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to TUN: %v", err)
	}
	return nil
}
