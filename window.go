package lzma

import "io"

type window struct {
	buf      []byte
	pos      uint32
	size     uint32
	isFull   bool
	TotalPos uint32

	pending uint32
}

func newWindow(dictSize uint32) *window {
	return &window{
		buf:      make([]byte, dictSize),
		pos:      0,
		TotalPos: 0,
		size:     dictSize,
		isFull:   false,
	}
}

func (w *window) PutByte(b byte) {
	w.TotalPos++
	w.buf[w.pos] = b
	w.pos++
	w.pending++

	if w.pos == w.size {
		w.pos = 0
		w.isFull = true
	}
}

func (w *window) GetByte(dist uint32) byte {
	i := w.size - dist + w.pos

	if dist <= w.pos {
		i = w.pos - dist
	}

	return w.buf[i]
}

func (w *window) CopyMatch(dist, len uint32) {
	for ; len > 0; len-- {
		w.PutByte(w.GetByte(dist))
	}
}

func (w *window) CheckDistance(dist uint32) bool {
	return dist <= w.pos || w.isFull
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

	for i := uint32(0); i < minLen; i++ {
		p[i] = w.GetByte(w.pending - i)
	}

	w.pending -= minLen

	return int(minLen), nil
}

func (w *window) Reset() {
	w.TotalPos = 0
	w.pos = 0
	w.isFull = false
	w.pending = 0
}

func (w *window) ReadFrom(r io.Reader) (n int64, err error) {
	var nn int

	nn, err = r.Read(w.buf[w.pos:])
	w.pos += uint32(nn)
	w.pending += uint32(nn)

	if w.pos == w.size {
		w.pos = 0
		w.isFull = true
	}

	return n, err
}

func (w *window) Available() int {
	if w.isFull {
		return 0
	}

	return int(w.size - w.pos)
}
