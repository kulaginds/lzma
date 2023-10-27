package lzma

type bitTreeDecoder struct {
	probs   []uint16
	numBits int
}

func newBitTreeDecoder(numBits int) *bitTreeDecoder {
	d := &bitTreeDecoder{
		numBits: numBits,
		probs:   make([]uint16, uint(1)<<numBits),
	}
	d.Reset()

	return d
}

func (d *bitTreeDecoder) Reset() {
	initProbs(d.probs)
}

func (d *bitTreeDecoder) Decode(rc *rangeDecoder) uint32 {
	m := uint32(1)

	for i := 0; i < d.numBits; i++ {
		m = (m << 1) + rc.DecodeBit(&d.probs[m])
	}

	return m - (uint32(1) << d.numBits)
}

func (d *bitTreeDecoder) ReverseDecode(rc *rangeDecoder) uint32 {
	return BitTreeReverseDecode(d.probs, d.numBits, rc)
}

func BitTreeReverseDecode(probs []uint16, numBits int, rc *rangeDecoder) uint32 {
	var bit uint32

	m := uint32(1)
	symbol := uint32(0)

	for i := 0; i < numBits; i++ {
		bit = rc.DecodeBit(&probs[m])

		m <<= 1
		m += bit
		symbol |= bit << i
	}

	return symbol
}
