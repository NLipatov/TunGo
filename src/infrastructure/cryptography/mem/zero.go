package mem

import "runtime"

// ZeroBytes overwrites a byte slice with zeros.
//
// SECURITY INVARIANT: This function MUST NOT be optimized away by the compiler.
// We use runtime.KeepAlive to create a happens-before edge that prevents
// dead-store elimination. The slice is considered "live" until after zeroing.
//
// LIMITATION: Go GC may have copied the slice before this call. This is
// best-effort defense against memory forensics, not a guarantee.
func ZeroBytes(b []byte) {
	if len(b) == 0 {
		return
	}
	for i := range b {
		b[i] = 0
	}
	// Prevent compiler from eliminating the zeroing as a dead store.
	runtime.KeepAlive(b)
}
