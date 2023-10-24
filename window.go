package lzma

import "io"

type window struct {
	outStream io.Writer

	buf    []byte
	pos    uint32
	size   uint32
	isFull bool

	TotalPos uint32
}

func newWindow(outStream io.Writer, dictSize uint32) *window {
	return &window{
		outStream: outStream,

		buf:    make([]byte, dictSize),
		pos:    0,
		size:   dictSize,
		isFull: false,

		TotalPos: 0,
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

	_, err := w.outStream.Write([]byte{b})
	if err != nil {
		return err
	}

	return nil
}

func (w *window) GetByte(dist uint32) byte {
	i := w.size - dist + w.pos

	if dist <= w.pos {
		i = w.pos - dist
	}

	return w.buf[i]
}

func (w *window) CopyMatch(dist, len uint32) error {
	var err error

	for ; len > 0; len-- {
		err = w.PutByte(w.GetByte(dist))
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *window) CheckDistance(dist uint32) bool {
	return dist <= w.pos || w.isFull
}

func (w *window) IsEmpty() bool {
	return w.pos == 0 && !w.isFull
}
