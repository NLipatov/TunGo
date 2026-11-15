package tcp

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"tungo/internal/protocol/chacha20"
	"tungo/internal/server/session"
)

const testWaitTimeout = time.Second

// recordingWriteCloser hands every written frame to packets and reports Close via closed.
type recordingWriteCloser struct {
	packets   chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	err       error
}

func newRecordingWriteCloser() *recordingWriteCloser {
	return &recordingWriteCloser{
		packets: make(chan []byte, 128),
		closed:  make(chan struct{}),
	}
}

func (w *recordingWriteCloser) Write(packet []byte) (int, error) {
	w.packets <- append([]byte(nil), packet...)
	if w.err != nil {
		return 0, w.err
	}
	return len(packet), nil
}

func (w *recordingWriteCloser) Close() error {
	w.closeOnce.Do(func() { close(w.closed) })
	return nil
}

// blockingWriteCloser blocks every Write until Close is called.
type blockingWriteCloser struct {
	writing   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func newBlockingWriteCloser() *blockingWriteCloser {
	return &blockingWriteCloser{writing: make(chan struct{}, 1), closed: make(chan struct{})}
}

func (w *blockingWriteCloser) Write([]byte) (int, error) {
	w.writing <- struct{}{}
	<-w.closed
	return 0, io.ErrClosedPipe
}

func (w *blockingWriteCloser) Close() error {
	w.closeOnce.Do(func() { close(w.closed) })
	return nil
}

// epochCrypto tags each frame with the epoch active at encryption time.
type epochCrypto struct {
	epoch byte
}

func (c *epochCrypto) Encrypt(frame []byte) ([]byte, error) {
	return append(frame, c.epoch), nil
}

type failingEncryptCrypto struct{}

func (failingEncryptCrypto) Encrypt([]byte) ([]byte, error) {
	return nil, errors.New("nonce exhausted")
}

func startPeerSender(t *testing.T, writer io.WriteCloser, crypto interface {
	Encrypt([]byte) ([]byte, error)
}) *peerSender {
	t.Helper()
	sender := newPeerSender(writer, crypto)
	go sender.processSendQueue()
	t.Cleanup(func() { _ = sender.Close() })
	return sender
}

func waitPacket(t *testing.T, packets <-chan []byte) []byte {
	t.Helper()
	select {
	case packet := <-packets:
		return packet
	case <-time.After(testWaitTimeout):
		t.Fatal("timed out waiting for written packet")
		return nil
	}
}

func waitClosed(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(testWaitTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestPeerSender_SendReservesEpochPrefixAndKeepsOrder(t *testing.T) {
	writer := newRecordingWriteCloser()
	sender := startPeerSender(t, writer, &plaintextCrypto{})
	payloads := [][]byte{[]byte("first"), []byte("second"), []byte("third")}

	for _, payload := range payloads {
		if err := sender.Send(payload); err != nil {
			t.Fatalf("Send(%q): %v", payload, err)
		}
	}
	for _, want := range payloads {
		got := waitPacket(t, writer.packets)
		if !bytes.Equal(got[:chacha20.TCPEpochPrefixSize], make([]byte, chacha20.TCPEpochPrefixSize)) {
			t.Fatalf("epoch prefix = %x, want reserved zeros", got[:chacha20.TCPEpochPrefixSize])
		}
		if !bytes.Equal(got[chacha20.TCPEpochPrefixSize:], want) {
			t.Fatalf("payload = %q, want %q", got[chacha20.TCPEpochPrefixSize:], want)
		}
	}
}

func TestPeerSender_SendCopiesPayload(t *testing.T) {
	writer := newRecordingWriteCloser()
	sender := newPeerSender(writer, &plaintextCrypto{})
	payload := []byte("original")

	if err := sender.Send(payload); err != nil {
		t.Fatal(err)
	}
	copy(payload, "mutated!")
	go sender.processSendQueue()
	t.Cleanup(func() { _ = sender.Close() })

	if got := waitPacket(t, writer.packets); string(got[chacha20.TCPEpochPrefixSize:]) != "original" {
		t.Fatalf("payload = %q, want copy taken at Send", got[chacha20.TCPEpochPrefixSize:])
	}
}

func TestPeerSender_SendDropsWithoutBlockingWhenQueueFull(t *testing.T) {
	sender := newPeerSender(newRecordingWriteCloser(), &plaintextCrypto{})

	for range cap(sender.dataq) {
		if err := sender.Send([]byte("x")); err != nil {
			t.Fatalf("Send while queue has room: %v", err)
		}
	}
	if err := sender.Send([]byte("x")); !errors.Is(err, errSendQueueFull) {
		t.Fatalf("Send on full queue error = %v, want %v", err, errSendQueueFull)
	}
}

func TestPeerSender_RejectsOversizedPayload(t *testing.T) {
	sender := newPeerSender(newRecordingWriteCloser(), &plaintextCrypto{})
	oversized := make([]byte, frameCap-chacha20.TCPOverhead+1)

	if err := sender.Send(oversized); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("Send error = %v, want %v", err, io.ErrShortBuffer)
	}
	if err := sender.SendControl(oversized, nil); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("SendControl error = %v, want %v", err, io.ErrShortBuffer)
	}
}

func TestPeerSender_ControlAfterRunsBeforeNextEncryption(t *testing.T) {
	writer := newRecordingWriteCloser()
	crypto := &epochCrypto{}
	sender := startPeerSender(t, writer, crypto)

	if err := sender.SendControl([]byte("ack"), func() { crypto.epoch = 1 }); err != nil {
		t.Fatal(err)
	}
	ack := waitPacket(t, writer.packets)
	if err := sender.Send([]byte("data")); err != nil {
		t.Fatal(err)
	}
	data := waitPacket(t, writer.packets)

	if epoch := ack[len(ack)-1]; epoch != 0 {
		t.Fatalf("ack encrypted with epoch %d, want 0", epoch)
	}
	if epoch := data[len(data)-1]; epoch != 1 {
		t.Fatalf("data after ack encrypted with epoch %d, want 1", epoch)
	}
}

func TestPeerSender_CloseUnblocksPendingWrite(t *testing.T) {
	writer := newBlockingWriteCloser()
	sender := newPeerSender(writer, &plaintextCrypto{})
	go sender.processSendQueue()

	if err := sender.Send([]byte("x")); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, writer.writing, "write to start")

	closed := make(chan struct{})
	go func() {
		_ = sender.Close()
		close(closed)
	}()
	waitClosed(t, closed, "Close to return while write was blocked")
}

func TestPeerSender_CloseIsIdempotent(t *testing.T) {
	sender := newPeerSender(newRecordingWriteCloser(), &plaintextCrypto{})
	go sender.processSendQueue()

	if err := sender.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sender.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sender.Send([]byte("x")); !errors.Is(err, session.ErrPeerClosed) {
		t.Fatalf("Send after Close error = %v, want %v", err, session.ErrPeerClosed)
	}
	if err := sender.SendControl([]byte("x"), nil); !errors.Is(err, session.ErrPeerClosed) {
		t.Fatalf("SendControl after Close error = %v, want %v", err, session.ErrPeerClosed)
	}
}

func TestPeerSender_FailureClosesTransport(t *testing.T) {
	for _, tt := range []struct {
		name    string
		crypto  interface{ Encrypt([]byte) ([]byte, error) }
		err     error
		wantErr error
	}{
		{name: "encryption", crypto: failingEncryptCrypto{}, wantErr: session.ErrEncryption},
		{name: "transport write", crypto: &plaintextCrypto{}, err: io.ErrClosedPipe, wantErr: session.ErrTransportWrite},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writer := newRecordingWriteCloser()
			writer.err = tt.err
			sender := newPeerSender(writer, tt.crypto)
			go sender.processSendQueue()

			if err := sender.Send([]byte("x")); err != nil {
				t.Fatal(err)
			}
			waitClosed(t, writer.closed, "transport close")
			waitClosed(t, sender.done, "writer exit")

			if !errors.Is(sender.err, tt.wantErr) {
				t.Fatalf("writer error = %v, want %v", sender.err, tt.wantErr)
			}
			if err := sender.Send([]byte("x")); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Send after failure error = %v, want %v", err, tt.wantErr)
			}
			if err := sender.SendControl([]byte("x"), nil); !errors.Is(err, tt.wantErr) {
				t.Fatalf("SendControl after failure error = %v, want %v", err, tt.wantErr)
			}
			if err := sender.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
