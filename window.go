package lzma

import (
	"io"
)

type window struct {
	buf     []byte
	pos     uint32
	size    uint32
	pending uint32
	isFull  bool
	//TotalPos uint32
}

func newWindow(dictSize uint32) *window {
	w := &window{
		buf: make([]byte, dictSize),
		pos: 0,
		//TotalPos: 0,
		size:   dictSize,
		isFull: false,
	}

	return w
}

func (w *window) PutByte(b byte) {
	//w.TotalPos++
	w.buf[w.pos] = b
	w.pos++
	w.pending++

	if w.pos >= w.size {
		w.pos -= w.size
		w.isFull = true
	}
}

func (w *window) GetByte(dist uint32) byte {
	i := w.pos - dist

	if dist > w.pos {
		i = w.size - dist + w.pos
	}

	return w.buf[i]
}

func (w *window) CopyMatch(dist, len uint32) {
	from := uint32(0)
	to := w.pos
	limit := w.size

	if dist <= w.pos {
		from += w.pos - dist
	} else {
		from += w.size - dist + w.pos
	}

	w.pos += len
	w.pending += len
	if w.pos >= w.size {
		w.pos -= w.size
		w.isFull = true
	}

	for ; len > 0; len-- {
		w.buf[to] = w.buf[from]
		from++
		to++

		if from == limit {
			from -= w.size
		}

		if to == limit {
			to -= w.size
		}
	}
}

func (w *window) CheckDistance(dist uint32) bool {
	return w.isFull || dist <= w.pos
}

func (w *window) IsEmpty() bool {
	return w.pos == 0 && !w.isFull
}

func (w *window) HasPending() bool {
	return w.pending > 0
}

func (w *window) ReadPending(p []byte) (int, error) {
	minLen := w.pending
	if uint32(len(p)) < minLen {
		minLen = uint32(len(p))
	}

	fromPtr := uint32(0)
	toPtr := uint32(0)
	fromLimit := uint32(len(w.buf))
	dist := w.pending
	if dist > w.pos {
		fromPtr += w.size - dist + w.pos
	} else {
		fromPtr += w.pos - dist
	}

	for i := uint32(0); i < minLen; i++ {
		p[toPtr] = w.buf[fromPtr]
		fromPtr++
		toPtr++

		if fromPtr == fromLimit {
			fromPtr -= w.size
		}
	}

	w.pending -= minLen

	return int(minLen), nil
}

func (w *window) Reset() {
	//w.TotalPos = 0
	w.pos = 0
	w.isFull = false
	w.pending = 0
}

func (w *window) ReadFrom(r io.Reader) (n int64, err error) {
	var nn int

	nn, err = r.Read(w.buf[w.pos:])
	w.pos += uint32(nn)
	w.pending += uint32(nn)

	if w.pos >= w.size {
		w.pos -= w.size
		w.isFull = true
	}

	return n, err
}

func (w *window) Available() int {
	return int(w.size - w.pending)
}
