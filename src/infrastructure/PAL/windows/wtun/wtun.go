//go:build windows

package wtun

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"

	"tungo/application/network/routing/tun"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
)

// ========================================================================================
// High-compat Wintun adapter with per-session refcount RCU and zero-spin drains.
// Uses only official wintun-go API (no unsafe layout assumptions).
// Hot path has no mutexes and no sleeps; reopen/close wait via OS events.
// ========================================================================================

const ringSize = 8 << 20 // 8 MiB (within wintun's RingCapacityMin..Max)

// Ensure interface conformance at compile time.
var _ tun.Device = (*TUN)(nil)

// sessionRef pairs a Wintun session with an in-flight counter and an OS event
// that is signaled when inflight becomes zero while draining is requested.
type sessionRef struct {
	s         *wintun.Session
	inflight  atomic.Int64   // pinned ops on this exact session
	drainWait atomic.Uint32  // 0: normal, 1: reopen/close is waiting
	zeroEvent windows.Handle // manual-reset event
}

// TUN is a high-performance Wintun adapter using per-session RCU swaps:
//  1. Start new session and publish it.
//  2. Arm old session for drain (drainWait=1).
//  3. If inflight>0, wait on old.zeroEvent (OS-level, no spin).
//  4. End() old session.
type TUN struct {
	adapter    *wintun.Adapter
	closeEvent windows.Handle // manual-reset to wake all readers on Close()
	cur        atomic.Pointer[sessionRef]
	closed     atomic.Bool
	reopenMu   sync.Mutex // serialize reopen/close (rare path)
}

// NewTUN creates the device and starts an initial Wintun session.
func NewTUN(adapter *wintun.Adapter) (tun.Device, error) {
	// Manual-reset event to wake ALL potential waiters on Close().
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("create close event: %w", err)
	}
	sess, err := adapter.StartSession(ringSize)
	if err != nil {
		_ = windows.CloseHandle(ev)
		return nil, fmt.Errorf("start session: %w", err)
	}
	zero, err := windows.CreateEvent(nil, 1, 0, nil) // manual-reset; we'll ResetEvent before waiting
	if err != nil {
		sess.End()
		_ = windows.CloseHandle(ev)
		return nil, fmt.Errorf("create zeroEvent: %w", err)
	}
	ref := &sessionRef{s: &sess, zeroEvent: zero}
	t := &TUN{adapter: adapter, closeEvent: ev}
	t.cur.Store(ref)
	return t, nil
}

// beginOp pins the current session with a refcount increment, with RCU-safe retry.
func (t *TUN) beginOp() (*sessionRef, *wintun.Session, error) {
	for {
		if t.closed.Load() {
			return nil, nil, windows.ERROR_OPERATION_ABORTED
		}
		ref := t.cur.Load()
		if ref == nil {
			return nil, nil, windows.ERROR_INVALID_HANDLE
		}
		ref.inflight.Add(1)
		// Validate that ref is still current and device not closed after the increment.
		if ref == t.cur.Load() && !t.closed.Load() {
			return ref, ref.s, nil
		}
		// Cur swapped or device closed — rollback and retry.
		if ref.inflight.Add(-1) == 0 && ref.drainWait.Load() != 0 {
			_ = windows.SetEvent(ref.zeroEvent)
		}
	}
}

// endOp decrements refcount and signals zeroEvent if a drain is armed.
func (t *TUN) endOp(ref *sessionRef) {
	if ref.inflight.Add(-1) == 0 && ref.drainWait.Load() != 0 {
		_ = windows.SetEvent(ref.zeroEvent)
	}
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

// reopenSession performs a per-session RCU swap without spin:
// publish new session; arm old for drain; wait on its zeroEvent if needed; End old.
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
	newZero, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		newSess.End()
		return fmt.Errorf("create zeroEvent(new): %w", err)
	}
	newRef := &sessionRef{s: &newSess, zeroEvent: newZero}
	t.cur.Store(newRef)

	if oldRef != nil {
		oldRef.drainWait.Store(1)
		// Fast path: if already zero, no wait; otherwise block in kernel.
		if oldRef.inflight.Load() != 0 {
			_ = windows.ResetEvent(oldRef.zeroEvent)
			_, werr := windows.WaitForSingleObject(oldRef.zeroEvent, windows.INFINITE)
			if werr != nil {
				// Still try to End() to avoid leaks.
				oldRef.s.End()
				_ = windows.CloseHandle(oldRef.zeroEvent)
				return werr
			}
		}
		oldRef.s.End()
		_ = windows.CloseHandle(oldRef.zeroEvent)
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

		switch rerr {
		case windows.ERROR_NO_MORE_ITEMS:
			// RX ring empty: wait on the *current* session (not on old ref),
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

		case windows.ERROR_HANDLE_EOF, windows.ERROR_INVALID_DATA:
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
// without global locks. The call returns only after the packet is queued or a
// terminal error occurs (including Close()).
func (t *TUN) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Optional: fast guard if caller accidentally sends jumbo frames.
	if len(p) > int(wintun.PacketSizeMax) {
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
			// TX ring full: short OS-yield; escalate to 1ms sleep under sustained pressure.
			if backoff < 2 {
				_ = windows.SleepEx(0, false) // give up timeslice
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

	// Unpublish and drain exactly that session (no spin).
	oldRef := t.cur.Swap(nil)
	if oldRef != nil {
		oldRef.drainWait.Store(1)
		if oldRef.inflight.Load() != 0 {
			_ = windows.ResetEvent(oldRef.zeroEvent)
			_, _ = windows.WaitForSingleObject(oldRef.zeroEvent, windows.INFINITE)
		}
		oldRef.s.End()
		_ = windows.CloseHandle(oldRef.zeroEvent)
	}

	_ = t.adapter.Close()
	_ = windows.CloseHandle(t.closeEvent)
	return nil
}
