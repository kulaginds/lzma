package lzma

type windowWithPending struct {
	buf    []byte
	pos    uint32
	size   uint32
	isFull bool

	TotalPos uint32
	pending  uint32
}

func newWindowWithPending(dictSize uint32) *windowWithPending {
	return &windowWithPending{
		buf:      make([]byte, dictSize),
		pos:      0,
		size:     dictSize,
		isFull:   false,
		TotalPos: 0,
	}
}

func (w *windowWithPending) PutByte(b byte) {
	w.TotalPos++
	w.buf[w.pos] = b
	w.pos++
	w.pending++

	if w.pos == w.size {
		w.pos = 0
		w.isFull = true
	}
}

func (w *windowWithPending) GetByte(dist uint32) byte {
	i := w.size - dist + w.pos

	if dist <= w.pos {
		i = w.pos - dist
	}

	return w.buf[i]
}

func (w *windowWithPending) CopyMatch(dist, len uint32) {
	for ; len > 0; len-- {
		w.PutByte(w.GetByte(dist))
	}
}

func (w *windowWithPending) CheckDistance(dist uint32) bool {
	return dist <= w.pos || w.isFull
}

func (w *windowWithPending) IsEmpty() bool {
	return w.pos == 0 && !w.isFull
}

func (w *windowWithPending) HasPending() bool {
	return w.pending > 0
}

func (w *windowWithPending) ReadPending(p []byte) (int, error) {
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
