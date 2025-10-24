//go:build windows

package wtun

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"

	"tungo/application/network/routing/tun"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
)

// ========================================================================================
// High-compat Wintun adapter with per-session refcount RCU.
// No mutexes on the hot path; official wintun-go API (no unsafe layout assumptions).
// ========================================================================================

const ringSize = 8 << 20 // 8 MiB (within RingCapacityMin..RingCapacityMax)

// Ensure interface conformance at compile time.
var _ tun.Device = (*TUN)(nil)

// sessionRef pairs a Wintun session with an in-flight counter.
// Readers/writers pin this exact session (no epoch ambiguity).
type sessionRef struct {
	s        *wintun.Session
	inflight atomic.Int64
}

// TUN is a high-performance Wintun adapter using per-session RCU swaps:
//  1. New session is created and published into cur.
//  2. Old session is ended only after its refcount drains to zero.
type TUN struct {
	adapter    *wintun.Adapter
	closeEvent windows.Handle

	// cur holds the currently active session ref.
	cur atomic.Pointer[sessionRef]

	// closed is set when Close() is called; prevents new operations.
	closed atomic.Bool

	// reopenMu serializes slow-path reopens/close. Hot path never takes it.
	reopenMu sync.Mutex
}

// NewTUN creates the device and starts an initial Wintun session.
func NewTUN(adapter *wintun.Adapter) (tun.Device, error) {
	// Manual-reset event to wake ALL potential waiters on Close().
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	// Optionally clamp to wintun.RingCapacityMin/Max; our value is within bounds.
	sess, err := adapter.StartSession(ringSize)
	if err != nil {
		_ = windows.CloseHandle(ev)
		return nil, fmt.Errorf("start session: %w", err)
	}
	ref := &sessionRef{s: &sess}
	t := &TUN{
		adapter:    adapter,
		closeEvent: ev,
	}
	t.cur.Store(ref)
	return t, nil
}

// beginOp pins the current session with a refcount increment.
// Returns the pinned ref and the session pointer to use.
func (t *TUN) beginOp() (*sessionRef, *wintun.Session, error) {
	if t.closed.Load() {
		return nil, nil, windows.ERROR_OPERATION_ABORTED
	}
	ref := t.cur.Load()
	if ref == nil {
		return nil, nil, windows.ERROR_INVALID_HANDLE
	}
	ref.inflight.Add(1)
	// After increment, ref is guaranteed to remain valid until endOp(ref).
	return ref, ref.s, nil
}

// endOp decrements the in-flight counter for the given ref.
func (t *TUN) endOp(ref *sessionRef) {
	ref.inflight.Add(-1)
}

// waitReadOrClose waits for either the session's read event or the device close event.
// Returns (closed=true) if the close event was signaled.
func (t *TUN) waitReadOrClose(readEvent windows.Handle, timeoutMs uint32) (closed bool, err error) {
	handles := []windows.Handle{readEvent, t.closeEvent}
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

// reopenSession performs a per-session RCU swap:
//  1. Start new session.
//  2. Publish &cur to the new ref.
//  3. Wait until old ref drains (inflight==0).
//  4. End() the old session.
func (t *TUN) reopenSession() error {
	t.reopenMu.Lock()
	defer t.reopenMu.Unlock()

	if t.closed.Load() {
		return windows.ERROR_OPERATION_ABORTED
	}

	oldRef := t.cur.Load()
	newSess, err := t.adapter.StartSession(ringSize)
	if err != nil {
		return err
	}
	newRef := &sessionRef{s: &newSess}
	t.cur.Store(newRef)

	if oldRef != nil {
		for oldRef.inflight.Load() != 0 {
			// Yield to other goroutines and give up OS timeslice without fixed delay.
			runtime.Gosched()
			_ = windows.SleepEx(0, false)
		}
		oldRef.s.End()
	}
	return nil
}

// Read reads a single packet into dst, blocking until one is available,
// or until the device is closed. It never silently truncates: if dst is too
// small, returns EMSGSIZE and drops the packet.
func (t *TUN) Read(dst []byte) (int, error) {
	for {
		if t.closed.Load() {
			return 0, windows.ERROR_OPERATION_ABORTED
		}
		ref, s, err := t.beginOp()
		if err != nil {
			return 0, err
		}

		packet, rerr := s.ReceivePacket()
		if rerr == nil {
			if len(packet) > len(dst) {
				// Do not silently truncate payload; drop packet and signal EMSGSIZE.
				s.ReleaseReceivePacket(packet)
				t.endOp(ref)
				return 0, syscall.EMSGSIZE
			}
			n := copy(dst, packet)
			s.ReleaseReceivePacket(packet)
			t.endOp(ref)
			return n, nil
		}
		t.endOp(ref)

		switch {
		case errors.Is(rerr, windows.ERROR_NO_MORE_ITEMS):
			// RX ring empty: wait on the *current* session (not on the old ref),
			// so reopen() cannot deadlock on us.
			curRef := t.cur.Load()
			if curRef == nil {
				continue
			}
			closed, werr := t.waitReadOrClose(curRef.s.ReadWaitEvent(), windows.INFINITE)
			if werr != nil {
				return 0, werr
			}
			if closed {
				return 0, windows.ERROR_OPERATION_ABORTED
			}
			continue
		case errors.Is(rerr, windows.ERROR_HANDLE_EOF), errors.Is(rerr, windows.ERROR_INVALID_DATA):
			// Session ended or ring corrupt: reopen and retry.
			if err := t.reopenSession(); err != nil {
				return 0, err
			}
			continue
		default:
			// Propagate unexpected kernel error to the caller.
			return 0, rerr
		}
	}
}

// Write writes a single packet. On ring saturation it uses a light adaptive backoff
// without taking global locks. The call returns only after the packet is queued or
// a terminal error occurs (including Close()).
func (t *TUN) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Optional hard guard: avoid surprising errors if callers pass jumbo frames.
	if len(p) > wintun.PacketSizeMax {
		return 0, syscall.EMSGSIZE
	}

	backoff := uint32(0) // grows a little under pressure
	for {
		if t.closed.Load() {
			return 0, windows.ERROR_OPERATION_ABORTED
		}
		ref, s, err := t.beginOp()
		if err != nil {
			return 0, err
		}

		buf, aerr := s.AllocateSendPacket(len(p))
		if aerr == nil {
			copy(buf, p)
			s.SendPacket(buf)
			t.endOp(ref)
			return len(p), nil
		}
		t.endOp(ref)

		switch {
		case errors.Is(aerr, windows.ERROR_HANDLE_EOF):
			// Session ended: reopen and try again.
			if err := t.reopenSession(); err != nil {
				return 0, err
			}
			continue

		case errors.Is(aerr, windows.ERROR_BUFFER_OVERFLOW):
			// TX ring full: light backoff (no write event is exposed by Wintun).
			if backoff < 2 {
				runtime.Gosched()
				_ = windows.SleepEx(0, false)
			} else {
				_ = windows.SleepEx(1, false)
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

// Close closes the device, interrupts any blocked Read() immediately, and
// drains the current session's in-flight operations before ending it.
func (t *TUN) Close() error {
	// Idempotent fast-path.
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Wake any waiters on waitReadOrClose().
	_ = windows.SetEvent(t.closeEvent)

	// Serialize with any concurrent reopen.
	t.reopenMu.Lock()
	defer t.reopenMu.Unlock()

	// Unpublish current ref and drain exactly that session.
	oldRef := t.cur.Swap(nil)
	if oldRef != nil {
		for oldRef.inflight.Load() != 0 {
			runtime.Gosched()
			_ = windows.SleepEx(0, false)
		}
		oldRef.s.End()
	}

	_ = t.adapter.Close()
	_ = windows.CloseHandle(t.closeEvent)
	return nil
}
