package tcp

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/server/session"
	transport "tungo/internal/transport/tcp"
)

var errSendQueueFull = errors.New("send queue full")

const (
	frameCap = settings.DefaultEthernetMTU + chacha20.TCPOverhead
	batchCap = 1024 * 64
)

type frame = [frameCap]byte

var pool = sync.Pool{
	New: func() any {
		return new(frame)
	},
}

type dataSendTask struct {
	data *frame
	n    int
}

type controlSendTask struct {
	data  *frame
	n     int
	after func()
}

type peerSender struct {
	writeCloser io.WriteCloser
	crypto      interface {
		Encrypt([]byte) ([]byte, error)
	}
	dataq         chan dataSendTask
	controlq      chan controlSendTask
	done, closing chan struct{}
	closeOnce     sync.Once
	err           error
}

func newPeerSender(
	writeCloser io.WriteCloser,
	crypto interface {
		Encrypt([]byte) ([]byte, error)
	}) *peerSender {
	return &peerSender{
		writeCloser: writeCloser,
		crypto:      crypto,
		controlq:    make(chan controlSendTask, 4),
		dataq:       make(chan dataSendTask, 1024),
		closing:     make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func (p *peerSender) Send(plaintext []byte) error {
	if len(plaintext)+chacha20.TCPOverhead > frameCap {
		return io.ErrShortBuffer
	}
	select {
	case <-p.done:
		return p.err
	case <-p.closing:
		return session.ErrPeerClosed
	default:
	}
	buf := pool.Get().(*frame)
	copied := copy(buf[chacha20.TCPEpochPrefixSize:], plaintext)
	select {
	case p.dataq <- dataSendTask{
		data: buf,
		n:    chacha20.TCPEpochPrefixSize + copied,
	}:
	default:
		pool.Put(buf)
		return errSendQueueFull
	}
	return nil
}

func (p *peerSender) SendControl(plaintext []byte, after func()) error {
	if len(plaintext)+chacha20.TCPOverhead > frameCap {
		return io.ErrShortBuffer
	}
	select {
	case <-p.done:
		return p.err
	case <-p.closing:
		return session.ErrPeerClosed
	default:
	}
	buf := pool.Get().(*frame)
	copied := copy(buf[chacha20.TCPEpochPrefixSize:], plaintext)
	select {
	case <-p.done:
		pool.Put(buf)
		return p.err
	case <-p.closing:
		pool.Put(buf)
		return session.ErrPeerClosed
	case p.controlq <- controlSendTask{
		data:  buf,
		n:     chacha20.TCPEpochPrefixSize + copied,
		after: after,
	}:
	}
	return nil
}

func (p *peerSender) Close() error {
	var err error
	p.closeOnce.Do(func() {
		close(p.closing)
		err = p.writeCloser.Close()
		<-p.done
	})
	return err
}

func (p *peerSender) processSendQueue() {
	defer close(p.done)
	// batch is owned by this goroutine and reused once each synchronous Write returns.
	batch := make([]byte, 0, batchCap)
	for {
		select {
		case <-p.closing:
			p.err = session.ErrPeerClosed
			return
		case task := <-p.dataq:
			if err := p.sendBatch(batch, task); err != nil {
				p.err = err
				_ = p.writeCloser.Close()
				return
			}
		case task := <-p.controlq:
			err := p.encryptAndWrite(batch, task.data[:task.n])
			pool.Put(task.data)
			if err != nil {
				p.err = err
				_ = p.writeCloser.Close()
				return
			}
			if task.after != nil {
				task.after()
			}
		}
	}
}

func (p *peerSender) encryptAndWrite(batch, plaintext []byte) error {
	frame, err := p.appendFrame(batch[:0], plaintext)
	if err != nil {
		return err
	}
	if _, err = p.writeCloser.Write(frame); err != nil {
		return fmt.Errorf("%w: %w", session.ErrTransportWrite, err)
	}
	return nil
}

func (p *peerSender) sendBatch(batch []byte, task dataSendTask) error {
	var err error
	var drain bool
	batch, err = p.appendFrame(batch[:0], task.data[:task.n])
	pool.Put(task.data)
	for err == nil &&
		!drain &&
		len(batch)+transport.LengthPrefix+frameCap <= cap(batch) {
		select {
		case task = <-p.dataq:
			batch, err = p.appendFrame(batch, task.data[:task.n])
			pool.Put(task.data)
		default:
			drain = true
		}
	}
	if err != nil {
		return err
	}
	if _, err = p.writeCloser.Write(batch); err != nil {
		return fmt.Errorf("%w: %w", session.ErrTransportWrite, err)
	}
	return nil
}

func (p *peerSender) appendFrame(batch, data []byte) ([]byte, error) {
	ciphertext, err := p.crypto.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", session.ErrEncryption, err)
	}
	return transport.AppendFrame(batch, ciphertext), nil
}
