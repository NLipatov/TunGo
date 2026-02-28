package server

import (
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"

	"tungo/infrastructure/settings"
)

// cfgNoIPv4Addr has a valid IPv4Subnet but no IPv4 address set,
// so IPv4CIDR() will return an error.
var cfgNoIPv4Addr = settings.Settings{
	Addressing: settings.Addressing{
		TunName:    "tun0",
		IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
	},
	MTU: settings.SafeMTU,
}

func newDeviceManager(ipMock *TunFactoryMockIP, ioctlMock *TunFactoryMockIOCTL) tunDeviceManager {
	return tunDeviceManager{ip: ipMock, ioctl: ioctlMock}
}

func TestTunDeviceManager_Create(t *testing.T) {
	injErr := errors.New("injected")

	t.Run("success IPv4 only", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		io := &TunFactoryMockIOCTL{}
		dm := newDeviceManager(ip, io)

		f, err := dm.create(baseCfg, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Fatal("expected non-nil file")
		}
		f.Close()

		log := ip.log.String()
		if !strings.Contains(log, "add;") {
			t.Errorf("expected TunTapAddDevTun call, got: %s", log)
		}
		if !strings.Contains(log, "up;") {
			t.Errorf("expected LinkSetDevUp call, got: %s", log)
		}
		if !strings.Contains(log, "mtu;") {
			t.Errorf("expected LinkSetDevMTU call, got: %s", log)
		}
		if !strings.Contains(log, "addr;") {
			t.Errorf("expected AddrAddDev call, got: %s", log)
		}
	})

	t.Run("success dual-stack", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		io := &TunFactoryMockIOCTL{}
		dm := newDeviceManager(ip, io)

		f, err := dm.create(baseCfgIPv6, true, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Fatal("expected non-nil file")
		}
		f.Close()

		// Should have two addr calls (IPv4 + IPv6).
		if count := strings.Count(ip.log.String(), "addr;"); count != 2 {
			t.Errorf("expected 2 AddrAddDev calls, got %d", count)
		}
	})

	t.Run("success IPv6 only", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		io := &TunFactoryMockIOCTL{}
		dm := newDeviceManager(ip, io)

		f, err := dm.create(baseCfgIPv6Only, false, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Fatal("expected non-nil file")
		}
		f.Close()

		if count := strings.Count(ip.log.String(), "addr;"); count != 1 {
			t.Errorf("expected 1 AddrAddDev call, got %d", count)
		}
	})

	t.Run("TunTapAddDevTun error", func(t *testing.T) {
		ip := &TunFactoryMockIPErr{
			TunFactoryMockIP: &TunFactoryMockIP{},
			errTag:           "TunTapAddDevTun",
			err:              injErr,
		}
		io := &TunFactoryMockIOCTL{}
		dm := tunDeviceManager{ip: ip, ioctl: io}

		_, err := dm.create(baseCfg, true, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "could not create tuntap dev") {
			t.Errorf("unexpected error message: %v", err)
		}
		// No rollback expected because created is still false.
		if strings.Contains(ip.TunFactoryMockIP.log.String(), "del;") {
			// The initial "delete previous tun" call is expected, but there
			// should be only that one del call (before the error), not a rollback del.
			count := strings.Count(ip.TunFactoryMockIP.log.String(), "del;")
			if count > 1 {
				t.Errorf("unexpected rollback delete, log: %s", ip.TunFactoryMockIP.log.String())
			}
		}
	})

	t.Run("LinkSetDevUp error triggers rollback", func(t *testing.T) {
		ip := &TunFactoryMockIPErr{
			TunFactoryMockIP: &TunFactoryMockIP{},
			errTag:           "LinkSetDevUp",
			err:              injErr,
		}
		io := &TunFactoryMockIOCTL{}
		dm := tunDeviceManager{ip: ip, ioctl: io}

		_, err := dm.create(baseCfg, true, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "could not set tuntap dev up") {
			t.Errorf("unexpected error message: %v", err)
		}
		// After TunTapAddDevTun succeeds, created=true; rollback should call LinkDelete.
		// Log has: del (initial cleanup) + add + del (rollback) = 2 del calls.
		log := ip.TunFactoryMockIP.log.String()
		if strings.Count(log, "del;") < 2 {
			t.Errorf("expected rollback LinkDelete call, log: %s", log)
		}
	})

	t.Run("LinkSetDevMTU error triggers rollback", func(t *testing.T) {
		ip := &TunFactoryMockIPErr{
			TunFactoryMockIP: &TunFactoryMockIP{},
			errTag:           "SetMTU",
			err:              injErr,
		}
		io := &TunFactoryMockIOCTL{}
		dm := tunDeviceManager{ip: ip, ioctl: io}

		_, err := dm.create(baseCfg, true, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "could not set mtu on tuntap dev") {
			t.Errorf("unexpected error message: %v", err)
		}
		log := ip.TunFactoryMockIP.log.String()
		if strings.Count(log, "del;") < 2 {
			t.Errorf("expected rollback LinkDelete call, log: %s", log)
		}
	})

	t.Run("AddrAddDev error IPv4 triggers rollback", func(t *testing.T) {
		ip := &TunFactoryMockIPErr{
			TunFactoryMockIP: &TunFactoryMockIP{},
			errTag:           "AddrAddDev",
			err:              injErr,
		}
		io := &TunFactoryMockIOCTL{}
		dm := tunDeviceManager{ip: ip, ioctl: io}

		_, err := dm.create(baseCfg, true, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "failed to convert server ip to CIDR format") {
			t.Errorf("unexpected error message: %v", err)
		}
		log := ip.TunFactoryMockIP.log.String()
		if strings.Count(log, "del;") < 2 {
			t.Errorf("expected rollback LinkDelete call, log: %s", log)
		}
	})

	t.Run("AddrAddDev error IPv6 second call triggers rollback", func(t *testing.T) {
		ip := &TunFactoryMockIPErrNthAddr{
			TunFactoryMockIP: &TunFactoryMockIP{},
			failOnCall:       2, // first call (IPv4) succeeds, second (IPv6) fails
			err:              injErr,
		}
		io := &TunFactoryMockIOCTL{}
		dm := tunDeviceManager{ip: ip, ioctl: io}

		_, err := dm.create(baseCfgIPv6, true, true)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "failed to assign IPv6 to TUN") {
			t.Errorf("unexpected error message: %v", err)
		}
		log := ip.TunFactoryMockIP.log.String()
		if strings.Count(log, "del;") < 2 {
			t.Errorf("expected rollback LinkDelete call, log: %s", log)
		}
	})

	t.Run("CreateTunInterface error triggers rollback", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		io := &TunFactoryMockIOCTL{createErr: injErr}
		dm := newDeviceManager(ip, io)

		_, err := dm.create(baseCfg, true, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "failed to open TUN interface") {
			t.Errorf("unexpected error message: %v", err)
		}
		log := ip.log.String()
		if strings.Count(log, "del;") < 2 {
			t.Errorf("expected rollback LinkDelete call, log: %s", log)
		}
	})

	t.Run("invalid IPv4 CIDR no addr set", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		io := &TunFactoryMockIOCTL{}
		dm := newDeviceManager(ip, io)

		_, err := dm.create(cfgNoIPv4Addr, true, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "could not derive server IPv4 CIDR") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("both IPv4 and IPv6 disabled", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		io := &TunFactoryMockIOCTL{}
		dm := newDeviceManager(ip, io)

		_, err := dm.create(baseCfg, false, false)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "no tunnel IP configuration") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestTunDeviceManager_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		dm := tunDeviceManager{ip: ip}

		if err := dm.delete("tun0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ip.log.String(), "del;") {
			t.Error("expected LinkDelete call")
		}
	})

	t.Run("error propagation", func(t *testing.T) {
		injErr := errors.New("delete failed")
		ip := &TunFactoryMockIPErrDel{
			TunFactoryMockIP: &TunFactoryMockIP{},
			err:              injErr,
		}
		dm := tunDeviceManager{ip: ip}

		err := dm.delete("tun0")
		if !errors.Is(err, injErr) {
			t.Fatalf("expected injected error, got: %v", err)
		}
	})
}

func TestTunDeviceManager_DetectName(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		io := &TunFactoryMockIOCTL{name: "tun42"}
		dm := tunDeviceManager{ioctl: io}

		f, _ := os.Open(os.DevNull)
		defer f.Close()

		name, err := dm.detectName(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "tun42" {
			t.Errorf("expected tun42, got %s", name)
		}
	})

	t.Run("error propagation", func(t *testing.T) {
		injErr := errors.New("detect failed")
		io := &TunFactoryMockIOCTL{detectErr: injErr}
		dm := tunDeviceManager{ioctl: io}

		f, _ := os.Open(os.DevNull)
		defer f.Close()

		_, err := dm.detectName(f)
		if !errors.Is(err, injErr) {
			t.Fatalf("expected injected error, got: %v", err)
		}
	})
}

func TestTunDeviceManager_ExternalInterface(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ip := &TunFactoryMockIP{}
		dm := tunDeviceManager{ip: ip}

		iface, err := dm.externalInterface()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if iface != "eth0" {
			t.Errorf("expected eth0, got %s", iface)
		}
	})

	t.Run("error propagation", func(t *testing.T) {
		injErr := errors.New("route failed")
		ip := &TunFactoryMockIPRouteErr{
			TunFactoryMockIP: TunFactoryMockIP{},
			err:              injErr,
		}
		dm := tunDeviceManager{ip: ip}

		_, err := dm.externalInterface()
		if !errors.Is(err, injErr) {
			t.Fatalf("expected injected error, got: %v", err)
		}
	})
}
