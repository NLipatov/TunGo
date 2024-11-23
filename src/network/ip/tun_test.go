package ip

import (
	"os/exec"
	"testing"
)

func interfaceExists(ifName string) bool {
	cmd := exec.Command("ip", "link", "show", ifName)
	err := cmd.Run()
	return err == nil
}

func Test_CreateAndDeleteInterface(t *testing.T) {
	ifName, err := UpNewTun("testtun0")
	if err != nil {
		t.Fatalf("failed to create tunnel: %v", err)
	}

	if !interfaceExists(ifName) {
		t.Errorf("tunnel %s should be created: %s", ifName, err)
		return
	}

	_, err = LinkDelete(ifName)
	if err != nil {
		t.Errorf("failed to delete %s tunnel", err)
		return
	}

	if interfaceExists(ifName) {
		t.Errorf("tunnel %s should be deleted: %s", ifName, err)
	}
}

func Test_WriteAndReadFromTun(t *testing.T) {
	ifName, err := UpNewTun("rwtesttun0")
	if err != nil {
		t.Fatalf("failed to create interface %v: %v", ifName, err)
	}
	defer func() {
		_, err = LinkDelete(ifName)
		if err != nil {
			t.Fatalf("failed to delete interface %v: %v", ifName, err)
		}
	}()

	tun, err := OpenTunByName(ifName)
	if err != nil {
		t.Fatalf("failed to open interface %v: %v", ifName, err)
	}

	packet := []byte{
		0x45, 0x00, 0x00, 0x54, 0x00, 0x00, 0x40, 0x00, 0x40, 0x01, 0xf7, 0x63, 0xc0, 0xa8, 0x00, 0x01, // IPv4 Header
		0xc0, 0xa8, 0x00, 0x02, // Sender and receiver addresses sample
	}

	n, err := tun.Write(packet)
	if err != nil {
		t.Fatalf("error writing to TUN: %v", err)
	}

	if n < 1 {
		t.Fatalf("nothing was written to TUN")
	}

	data := make([]byte, 1500)
	n, err = tun.Read(data)
	if err != nil {
		t.Fatalf("error reading from TUN: %v", err)
	}

	if n < 1 {
		t.Fatalf("nothing was read from TUN")
	}
}
