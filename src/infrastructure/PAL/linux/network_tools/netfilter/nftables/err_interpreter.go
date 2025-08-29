package nftables

import (
	"errors"
	"strings"
	"syscall"
)

type ErrInterpreter interface {
	isNatUnsupported(err error) bool
	isAFNotSupported(err error) bool
	isSeqMismatch(err error) bool
	isAlreadyExists(err error) bool
	isTransientNetlink(err error) bool
}

type DefaultErrInterpreter struct {
}

func NewDefaultErrInterpreter() *DefaultErrInterpreter {
	return &DefaultErrInterpreter{}
}

func (d *DefaultErrInterpreter) isNatUnsupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return d.isAFNotSupported(err) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.EPROTONOSUPPORT) ||
		strings.Contains(s, "operation not supported") ||
		strings.Contains(s, "not supported by protocol")
}

func (d *DefaultErrInterpreter) isAFNotSupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EAFNOSUPPORT) ||
		strings.Contains(s, "address family not supported")
}

func (d *DefaultErrInterpreter) isSeqMismatch(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "mismatched sequence") ||
		strings.Contains(s, "sequence mismatch") ||
		strings.Contains(s, "wrong sequence")
}

func (d *DefaultErrInterpreter) isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EEXIST) ||
		strings.Contains(s, "file exists") ||
		strings.Contains(s, "already exists")
}

func (d *DefaultErrInterpreter) isTransientNetlink(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, syscall.EAGAIN) ||
		errors.Is(err, syscall.EBUSY) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ENOBUFS) ||
		errors.Is(err, syscall.EINTR) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ENETDOWN) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		strings.Contains(strings.ToLower(err.Error()), "resource busy") ||
		strings.Contains(strings.ToLower(err.Error()), "try again") ||
		strings.Contains(strings.ToLower(err.Error()), "timed out") ||
		strings.Contains(strings.ToLower(err.Error()), "no buffer space")
}
