package ip

import (
	"os"
	"os/exec"
	"testing"
	"time"
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

	_, err = LinkDel(ifName)
	if err != nil {
		t.Errorf("failed to delete %s tunnel", err)
		return
	}

	if interfaceExists(ifName) {
		t.Errorf("tunnel %s should be deleted: %s", ifName, err)
	}
}

func Test_WriteAndReadFromTun(t *testing.T) {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		t.Skip("/dev/net/tun is not available; skipping TUN test")
	}

	tunName := "rwtesttun0"
	_, _ = LinkDel(tunName)
	ifName, err := UpNewTun(tunName)
	if err != nil {
		t.Fatalf("failed to create interface %v: %v", ifName, err)
	}
	defer func() {
		_, err = LinkDel(ifName)
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

	// Run the read in a separate goroutine and use a channel to receive the result.
	resultCh := make(chan struct {
		n   int
		err error
	})

	data := make([]byte, 1500)
	go func() {
		read, readErr := tun.Read(data)
		resultCh <- struct {
			n   int
			err error
		}{read, readErr}
	}()

	// Wait for the read to complete or timeout.
	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("error reading from TUN: %v", res.err)
		}
		if res.n < 1 {
			t.Fatalf("nothing was read from TUN")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for TUN read")
	}
}
