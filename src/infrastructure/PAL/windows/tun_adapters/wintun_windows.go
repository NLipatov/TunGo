//go:build windows

package tun_adapters

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"tungo/application/network/routing/tun"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
)

// ======== Win32 / Wintun plumbing (low-level, zero-overhead) ========

var (
	modWintun                                   = windows.NewLazySystemDLL("wintun.dll")
	procReceivePacket, procReleaseReceivePacket *windows.LazyProc
	ErrInvalidHandle                            = errors.New("invalid handle")
)

func init() {
	if err := modWintun.Load(); err != nil {
		log.Fatalf("load wintun.dll: %v", err)
	}
	procReceivePacket = modWintun.NewProc("WintunReceivePacket")
	procReleaseReceivePacket = modWintun.NewProc("WintunReleaseReceivePacket")
}

// sessionHandle extracts the underlying HANDLE from a *wintun.Session.
// This relies on the current layout of wintun.Session. We keep all
// ring syscalls in one place so it's easy to update on upstream changes.
func sessionHandle(s *wintun.Session) uintptr {
	// Equivalent to *(*uintptr)(unsafe.Pointer(s))
	return *(*uintptr)(unsafe.Pointer(s))
}

func recvPacketPtr(s *wintun.Session) (ptr uintptr, size uint32, errno syscall.Errno) {
	h := sessionHandle(s)
	r1, _, e1 := syscall.SyscallN(
		procReceivePacket.Addr(),
		h,
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		errno = e1
		return
	}
	ptr = r1
	return
}

func releasePacketPtr(s *wintun.Session, ptr uintptr) {
	h := sessionHandle(s)
	_, _, _ = syscall.SyscallN(procReleaseReceivePacket.Addr(), h, ptr)
}

// ======== Adapter implementation ========

const ringSize = 8 << 20 // 8 MiB

// wintunTun is a high-performance Wintun adapter with RCU-style session swap.
// Hot path is lock-free (only atomics). Reopen/Close are synchronized.
type wintunTun struct {
	adapter    *wintun.Adapter
	closeEvent windows.Handle

	// cur holds the currently active session pointer.
	cur atomic.Pointer[wintun.Session]

	// inFlight counts active hot-path operations (Receive/Release or Allocate/Send).
	inFlight atomic.Int64

	// closed is set when Close() is called.
	closed atomic.Bool

	// reopenMu serializes slow-path reopens. Hot path never takes it.
	reopenMu sync.Mutex
}

func NewWinTun(adapter *wintun.Adapter) (tun.Device, error) {
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	sess, err := adapter.StartSession(ringSize)
	if err != nil {
		_ = windows.CloseHandle(ev)
		return nil, fmt.Errorf("start session: %w", err)
	}
	t := &wintunTun{
		adapter:    adapter,
		closeEvent: ev,
	}
	// Store session pointer (escapes to heap by design).
	t.cur.Store(&sess)
	return t, nil
}

// beginOp pins the current session and marks an in-flight operation.
// The returned session remains valid until endOp() due to RCU semantics.
func (t *wintunTun) beginOp() (*wintun.Session, error) {
	if t.closed.Load() {
		return nil, syscall.ERROR_OPERATION_ABORTED
	}
	t.inFlight.Add(1)
	sess := t.cur.Load()
	if sess == nil {
		t.inFlight.Add(-1)
		return nil, ErrInvalidHandle
	}
	return sess, nil
}

func (t *wintunTun) endOp() {
	t.inFlight.Add(-1)
}

// waitReadOrClose waits for either the session's read event to fire or the adapter to be closed.
// Returns (closed=true) when closeEvent is signaled.
func (t *wintunTun) waitReadOrClose(readEvent windows.Handle, timeoutMs uint32) (closed bool, err error) {
	handles := []windows.Handle{readEvent, t.closeEvent}
	// WAIT_OBJECT_0 + i indicates which handle triggered.
	status, werr := windows.WaitForMultipleObjects(handles, false, timeoutMs)
	if werr != nil {
		return false, werr
	}
	switch status {
	case windows.WAIT_OBJECT_0 + 0:
		return false, nil // data available
	case windows.WAIT_OBJECT_0 + 1:
		return true, nil // closed signaled
	case uint32(windows.WAIT_TIMEOUT):
		return false, nil
	default:
		return false, syscall.EINVAL
	}
}

// reopenSession performs an RCU-style session swap:
//  1. Create new session.
//  2. Publish it via cur.Store(new).
//  3. Wait until all in-flight ops on the old session drain.
//  4. End() the old session.
func (t *wintunTun) reopenSession() error {
	t.reopenMu.Lock()
	defer t.reopenMu.Unlock()

	if t.closed.Load() {
		return syscall.ERROR_OPERATION_ABORTED
	}

	old := t.cur.Load()
	// Fast path: if old is nil, just start.
	newSess, err := t.adapter.StartSession(ringSize)
	if err != nil {
		return err
	}
	t.cur.Store(&newSess)

	// Drain old users (hot path ops).
	if old != nil {
		for {
			if t.inFlight.Load() == 0 {
				break
			}
			// Yield, then light sleep if needed to avoid busy looping.
			runtime.Gosched()
			windows.SleepEx(0, true)
		}
		old.End()
	}
	return nil
}

func (t *wintunTun) Read(dst []byte) (int, error) {
	// We keep the API: blocking read, one packet -> copy into dst.
	// No mutex in hot path. We only increment/decrement inFlight.
	for {
		if t.closed.Load() {
			return 0, syscall.ERROR_OPERATION_ABORTED
		}
		sess, err := t.beginOp()
		if err != nil {
			return 0, err
		}

		ptr, sz, errno := recvPacketPtr(sess)
		if errno == 0 {
			// We must release the packet back to the same session before endOp().
			if int(sz) > len(dst) {
				// Do not silently truncate payload; drop packet and signal EMSGSIZE.
				releasePacketPtr(sess, ptr)
				t.endOp()
				return 0, syscall.EMSGSIZE
			}
			src := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), sz)
			n := copy(dst, src)
			releasePacketPtr(sess, ptr)
			t.endOp()
			return n, nil
		}
		t.endOp()

		switch errno {
		case windows.ERROR_NO_MORE_ITEMS:
			// Nothing to read now; wait on either data or close.
			// Use a reasonably long timeout; WAIT will return earlier on event.
			s := t.cur.Load()
			if s == nil {
				continue
			}
			closed, werr := t.waitReadOrClose(s.ReadWaitEvent(), 5000)
			if werr != nil {
				return 0, werr
			}
			if closed {
				return 0, syscall.ERROR_OPERATION_ABORTED
			}
			continue

		case windows.ERROR_HANDLE_EOF:
			// Session died; reopen and retry.
			if err := t.reopenSession(); err != nil {
				return 0, err
			}
			continue

		default:
			// Any other kernel error is fatal to the caller.
			return 0, errno
		}
	}
}

func (t *wintunTun) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Hot path: one Allocate -> copy -> Send. No mutex; rely on inFlight.
	// If ring is full, do adaptive backoff, still without global locks.
	backoff := uint32(0) // grows to small waits under pressure
	for {
		if t.closed.Load() {
			return 0, syscall.ERROR_OPERATION_ABORTED
		}
		sess, err := t.beginOp()
		if err != nil {
			return 0, err
		}

		buf, aerr := sess.AllocateSendPacket(len(p))
		if aerr == nil {
			copy(buf, p)
			sess.SendPacket(buf)
			t.endOp()
			return len(p), nil
		}
		t.endOp()

		// Handle ring full / EOF / transient.
		switch {
		case errors.Is(aerr, windows.ERROR_HANDLE_EOF):
			if err := t.reopenSession(); err != nil {
				return 0, err
			}
			continue

		case errors.Is(aerr, windows.ERROR_BUFFER_OVERFLOW) ||
			errors.Is(aerr, windows.ERROR_NO_MORE_ITEMS):
			// Ring is full: wait a bit (adaptive), or until closed.
			// Wintun doesn't expose a write wait event; we wait briefly.
			if backoff < 2 {
				// First retries: yield only.
				runtime.Gosched()
			} else {
				// Short sleep to avoid busy spin under sustained pressure.
				windows.SleepEx(1, true)
			}
			if backoff < 10 {
				backoff++
			}
			continue

		default:
			return 0, aerr
		}
	}
}

func (t *wintunTun) Close() error {
	// Idempotent, immediate wake of any blocked Read() via closeEvent.
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	_ = windows.SetEvent(t.closeEvent)

	// Swap out current session (publish nil) and wait for in-flight ops to finish,
	// then End() the old session.
	t.reopenMu.Lock()
	old := t.cur.Swap(nil)
	// Drain all hot-path users.
	for t.inFlight.Load() != 0 {
		runtime.Gosched()
		windows.SleepEx(0, true)
	}
	if old != nil {
		old.End()
	}
	t.reopenMu.Unlock()

	_ = t.adapter.Close()
	_ = windows.CloseHandle(t.closeEvent)
	return nil
}
