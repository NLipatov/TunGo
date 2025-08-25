package adapter

import "io"

// Copier drains readers. Default uses io.Copy.
type Copier interface {
	Copy(dst io.Writer, src io.Reader) (int64, error)
}

type DefaultCopier struct{}

func (DefaultCopier) Copy(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}
