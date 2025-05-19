package windows

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"log"
	"sync"
	"sync/atomic"
	"syscall"
	"tungo/application"
	"unsafe"
)

var (
	// modWintun is the handle to wintun.dll loaded lazily
	modWintun = windows.NewLazySystemDLL("wintun.dll")

	// addrRecvPacket and addrRelPacket store addresses of WintunReceivePacket and WintunReleaseReceivePacket
	addrRecvPacket, addrRelPacket uintptr
)

// ringSize (8 MiB) is the shared buffer size for Wintun; large enough for high throughput.
const ringSize = 0x800000 // 8 MiB

// init loads wintun.dll and initializes function pointers
func init() {
	if err := modWintun.Load(); err != nil {
		log.Fatalf("load wintun.dll: %v", err)
	}
	addrRecvPacket = modWintun.NewProc("WintunReceivePacket").Addr()
	addrRelPacket = modWintun.NewProc("WintunReleaseReceivePacket").Addr()
}

type wintunTun struct {
	adapter     *wintun.Adapter
	session     *wintun.Session
	sessionMu   sync.RWMutex
	reopenMutex sync.Mutex
	closeEvent  windows.Handle
	closed      atomic.Bool
}

func NewWinTun(adapter *wintun.Adapter) (application.TunDevice, error) {
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}

	sess, err := adapter.StartSession(ringSize)
	if err != nil {
		_ = windows.CloseHandle(ev)
		return nil, fmt.Errorf("start session: %w", err)
	}

	return &wintunTun{
		adapter:    adapter,
		session:    &sess,
		closeEvent: ev,
	}, nil
}

func (d *wintunTun) reopenSession() error {
	d.reopenMutex.Lock()
	defer d.reopenMutex.Unlock()

	if d.closed.Load() {
		return fmt.Errorf("device closed")
	}

	d.sessionMu.Lock()
	if d.session != nil {
		d.session.End()
	}
	d.sessionMu.Unlock()

	newSess, err := d.adapter.StartSession(ringSize)
	if err != nil {
		return err
	}

	d.sessionMu.Lock()
	d.session = &newSess
	d.sessionMu.Unlock()
	return nil
}

func (d *wintunTun) Read(dst []byte) (int, error) {
	for {
		if d.closed.Load() {
			return 0, fmt.Errorf("device closed")
		}

		d.sessionMu.RLock()
		sess := d.session
		d.sessionMu.RUnlock()

		ptr, sz, err := recvPacketPtr(sess)
		if err == nil {
			// Pointer received from external DLL; safe to cast to unsafe.Pointer.
			//noinspection GoVetUnsafePointer
			bytePointer := (*byte)(unsafe.Pointer(ptr))
			src := unsafe.Slice(bytePointer, sz)
			n := copy(dst, src)
			releasePacketPtr(sess, ptr)
			return n, nil
		}

		switch {
		case errors.Is(err, windows.ERROR_NO_MORE_ITEMS):
			if ret, werr := windows.WaitForSingleObject(sess.ReadWaitEvent(), 250); ret == windows.WAIT_FAILED || werr != nil {
				return 0, fmt.Errorf("session closed")
			}
		case errors.Is(err, windows.ERROR_HANDLE_EOF):
			if err := d.reopenSession(); err != nil {
				return 0, err
			}
		default:
			return 0, err
		}
	}
}

func recvPacketPtr(s *wintun.Session) (ptr uintptr, size uint32, err error) {
	h := *(*uintptr)(unsafe.Pointer(s))
	r1, _, e1 := syscall.SyscallN(
		addrRecvPacket,
		h,
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		err = e1
		return
	}
	ptr = r1
	return
}

func releasePacketPtr(s *wintun.Session, ptr uintptr) {
	h := *(*uintptr)(unsafe.Pointer(s))
	_, _, _ = syscall.SyscallN(addrRelPacket, h, ptr)
}

func (d *wintunTun) Write(p []byte) (int, error) {
	for {
		if d.closed.Load() {
			return 0, fmt.Errorf("device closed")
		}

		d.sessionMu.RLock()
		sess := d.session
		buf, err := sess.AllocateSendPacket(len(p))
		d.sessionMu.RUnlock()

		if err != nil {
			if errors.Is(err, windows.ERROR_HANDLE_EOF) {
				if err := d.reopenSession(); err != nil {
					return 0, err
				}
				continue
			}
			return 0, err
		}

		copy(buf, p)
		sess.SendPacket(buf)
		return len(p), nil
	}
}

func (d *wintunTun) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	_ = windows.SetEvent(d.closeEvent)

	d.sessionMu.Lock()
	if d.session != nil {
		d.session.End()
		d.session = nil
	}
	d.sessionMu.Unlock()

	_ = d.adapter.Close()
	_ = windows.CloseHandle(d.closeEvent)
	return nil
}
