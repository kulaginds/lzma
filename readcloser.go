package lzma

import (
	"errors"
	"fmt"
	"io"
)

type readCloser struct {
	c io.Closer
	r io.Reader
}

var errAlreadyClosed = errors.New("lzma: already closed")

func (rc *readCloser) Close() error {
	if rc.c == nil || rc.r == nil {
		return errAlreadyClosed
	}

	if err := rc.c.Close(); err != nil {
		return fmt.Errorf("lzma: error closing: %w", err)
	}

	rc.c, rc.r = nil, nil

	return nil
}

func (rc *readCloser) Read(p []byte) (int, error) {
	if rc.r == nil {
		return 0, errAlreadyClosed
	}

	n, err := rc.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = fmt.Errorf("lzma: error reading: %w", err)
	}

	return n, err
}
