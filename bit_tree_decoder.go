package lzma

type bitTreeDecoder struct {
	probs   []prob
	numBits int
}

func newBitTreeDecoder(numBits int) *bitTreeDecoder {
	d := &bitTreeDecoder{
		numBits: numBits,
		probs:   make([]prob, uint32(1)<<numBits),
	}
	d.Reset()

	return d
}

func (d *bitTreeDecoder) Reset() {
	initProbs(d.probs)
}

func (d *bitTreeDecoder) Decode(rc *rangeDecoder) (uint32, error) {
	m := uint32(1)

	var (
		bit uint32
		err error
	)

	for i := 0; i < d.numBits; i++ {
		bit, err = rc.DecodeBit(&d.probs[m])
		if err != nil {
			return 0, err
		}

		m = (m << 1) + bit
	}

	return m - (uint32(1) << d.numBits), nil
}

func (d *bitTreeDecoder) ReverseDecode(rc *rangeDecoder) (uint32, error) {
	return BitTreeReverseDecode(d.probs, d.numBits, rc)
}

func BitTreeReverseDecode(probs []prob, numBits int, rc *rangeDecoder) (uint32, error) {
	var (
		bit uint32
		err error
	)

	m := uint32(1)
	symbol := uint32(0)

	for i := 0; i < numBits; i++ {
		bit, err = rc.DecodeBit(&probs[m])
		if err != nil {
			return 0, err
		}

		m = (m << 1) | bit
		symbol |= bit << i
	}

	return symbol, nil
}
