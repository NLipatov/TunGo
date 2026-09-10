package iolifecycle

import (
	"testing"
	"time"
)

func TestDrainRejectsNewIO(t *testing.T) {
	gate := New()
	unblockCalled := false
	gate.Drain(func() { unblockCalled = true })

	if !gate.Closing() {
		t.Fatal("gate is not closing after Drain")
	}
	if unblockCalled {
		t.Fatal("Drain called unblockIO without active I/O")
	}
	if gate.TryAcquire() {
		t.Fatal("TryAcquire succeeded after Drain")
	}
}

func TestDrainUnblocksAndWaitsForActiveIO(t *testing.T) {
	gate := New()
	if !gate.TryAcquire() || !gate.TryAcquire() {
		t.Fatal("TryAcquire failed on an open gate")
	}

	unblockCalled := make(chan struct{})
	drainDone := make(chan struct{})
	go func() {
		gate.Drain(func() { close(unblockCalled) })
		close(drainDone)
	}()

	select {
	case <-unblockCalled:
	case <-time.After(time.Second):
		t.Fatal("Drain did not call unblockIO")
	}
	gate.Release()
	select {
	case <-drainDone:
		t.Fatal("Drain returned before the last Release")
	default:
	}
	gate.Release()
	select {
	case <-drainDone:
	case <-time.After(time.Second):
		t.Fatal("Drain did not return after the last Release")
	}
}

func BenchmarkAcquireRelease(b *testing.B) {
	gate := New()
	b.ReportAllocs()
	for b.Loop() {
		if !gate.TryAcquire() {
			b.Fatal("TryAcquire failed")
		}
		gate.Release()
	}
}
