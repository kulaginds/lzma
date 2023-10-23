package lzma

import (
	"io"
)

type window struct {
	outStream io.Writer

	buf    []byte
	pos    uint32
	size   uint32
	isFull bool

	TotalPos uint32

	cache      []byte
	cacheIndex int
}

func newWindow(outStream io.Writer, dictSize uint32) *window {
	return &window{
		outStream: outStream,

		buf:      make([]byte, dictSize),
		pos:      0,
		size:     dictSize,
		isFull:   false,
		TotalPos: 0,

		cache:      make([]byte, 65535),
		cacheIndex: -1,
	}
}

func (w *window) PutByte(b byte) error {
	w.TotalPos++
	w.buf[w.pos] = b
	w.pos++

	if w.pos == w.size {
		w.pos = 0
		w.isFull = true
	}

	if w.cacheIndex < 0 || w.cacheIndex >= len(w.cache) {
		_, err := w.outStream.Write(w.cache)
		if err != nil {
			return err
		}

		w.cacheIndex = 0
	}

	w.cache[w.cacheIndex] = b
	w.cacheIndex++

	return nil
}

func (w *window) CachePending() bool {
	return w.cacheIndex > 0
}

func (w *window) Flush() error {
	_, err := w.outStream.Write(w.cache)
	if err != nil {
		return err
	}

	w.cacheIndex = 0

	return nil
}

func (w *window) GetByte(dist uint32) byte {
	i := w.size - dist + w.pos
	if dist <= w.pos {
		i = w.pos - dist
	}

	return w.buf[i]
}

func (w *window) CopyMatch(dist, length uint32) error {
	var err error

	w.TotalPos += length
	tmp := w.buf[w.pos : w.pos+length]

	for ; length > 0; length-- {
		w.buf[w.pos] = w.GetByte(dist)
		w.pos++

		if w.pos == w.size {
			tmp = tmp[:len(tmp)-int(length)]
			_, err = w.outStream.Write(tmp)
			if err != nil {
				return err
			}

			w.pos = 0
			w.isFull = true
			tmp = w.buf[w.pos : w.pos+length]
		}
	}

	_, err = w.outStream.Write(tmp)
	if err != nil {
		return err
	}

	return nil
}

func (w *window) CheckDistance(dist uint32) bool {
	return dist <= w.pos || w.isFull
}

func (w *window) IsEmpty() bool {
	return w.pos == 0 && !w.isFull
}
