package lzma

import "fmt"

type bitTreeDecoder struct {
	rc *rangeDecoder

	probs   []uint16
	numBits int
}

func newBitTreeDecoder(rc *rangeDecoder, numBits int) *bitTreeDecoder {
	return &bitTreeDecoder{
		rc: rc,

		numBits: numBits,
		probs:   make([]uint16, uint(1)<<numBits),
	}
}

func (d *bitTreeDecoder) Init() {
	initProbs(d.probs)
}

func (d *bitTreeDecoder) Decode() (uint32, error) {
	var (
		b   uint32
		err error
	)

	m := uint32(1)

	for i := 0; i < d.numBits; i++ {
		b, err = d.rc.DecodeBit(&d.probs[m])
		if err != nil {
			return 0, fmt.Errorf("decode bit: %w", err)
		}

		m = (m << 1) + b
	}

	return m - (uint32(1) << d.numBits), nil
}

func (d *bitTreeDecoder) ReverseDecode() (uint32, error) {
	return BitTreeReverseDecode(d.probs, d.numBits, d.rc)
}

func BitTreeReverseDecode(probs []uint16, numBits int, rc *rangeDecoder) (uint32, error) {
	var (
		bit uint32
		err error
	)

	m := uint32(1)
	symbol := uint32(0)

	for i := 0; i < numBits; i++ {
		bit, err = rc.DecodeBit(&probs[m])
		if err != nil {
			return 0, fmt.Errorf("decode bit: %w", err)
		}

		m <<= 1
		m += bit
		symbol |= bit << i
	}

	return symbol, nil
}
