//go:build unix
// +build unix

package ip

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"strings"
	"unsafe"
)

const (
	ifNamSiz  = 16         // Max if name size, bytes
	tunSetIff = 0x400454ca // Code to create TUN/TAP if via ioctl
	iffTun    = 0x0001     // Enabling TUN flag
	IffNoPi   = 0x1000     // Disabling PI (Packet Information)
)

func UpNewTun(ifName string) (string, error) {
	err := enableIPv4Forwarding()
	if err != nil {
		return "", err
	}

	_, err = LinkAdd(ifName)
	if err != nil {
		return "", err
	}

	_, err = LinkSetUp(ifName)
	if err != nil {
		return "", err
	}

	return ifName, nil
}

func OpenTunByName(ifName string) (*os.File, error) {
	tunFilePath := "/dev/net/tun"
	tun, err := os.OpenFile(tunFilePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %v: %v", tunFilePath, err)
	}

	var req IfReq
	copy(req.Name[:], ifName)
	req.Flags = iffTun | IffNoPi

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, tun.Fd(), uintptr(tunSetIff), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		_ = tun.Close()
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
