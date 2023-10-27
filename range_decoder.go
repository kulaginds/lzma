package lzma

import (
	"errors"
	"fmt"
	"io"
)

type rangeDecoder struct {
	inStream io.Reader

	Range     uint32
	Code      uint32
	Corrupted bool

	b, btmp      []byte
	bi           int
	bSwapped     bool
	bInitialized bool
}

func newRangeDecoder(inStream io.Reader) *rangeDecoder {
	return &rangeDecoder{
		inStream: inStream,

		Range: 0xFFFFFFFF,

		b:        make([]byte, lzmaRequiredInputMax),
		btmp:     make([]byte, lzmaRequiredInputMax),
		bSwapped: true,
	}
}

func (d *rangeDecoder) IsFinishedOK() bool {
	return d.Code == 0
}

func (d *rangeDecoder) Init() (bool, error) {
	header := make([]byte, rangeDecoderHeaderLen)

	n, err := d.inStream.Read(header)
	if err != nil {
		return false, fmt.Errorf("inStream.Read: %w", err)
	}

	if n != rangeDecoderHeaderLen {
		return false, ErrCorrupted
	}

	b := header[0]
	header = header[1:]

	for i := 0; i < len(header); i++ {
		d.Code = (d.Code << 8) | uint32(header[i])
	}

	if b != 0 || d.Code == d.Range {
		d.Corrupted = true
	}

	return b == 0, nil
}

func (d *rangeDecoder) WarmUp() error {
	var (
		n   int
		err error
	)

	if !d.bInitialized {
		n, err = d.inStream.Read(d.b)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		d.b = d.b[:n]
		d.bInitialized = true
	}

	if !d.bSwapped {
		return nil
	}

	d.btmp = d.btmp[:cap(d.btmp)]
	n, err = d.inStream.Read(d.btmp)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	d.btmp = d.btmp[:n]
	d.bSwapped = false

	return nil
}

func (d *rangeDecoder) DecodeBit(prob *uint16) uint32 {
	bound := (d.Range >> kNumBitModelTotalBits) * uint32(*prob)

	var symbol uint32

	if d.Code < bound {
		*prob += ((1 << kNumBitModelTotalBits) - *prob) >> kNumMoveBits
		d.Range = bound
		symbol = 0
	} else {
		*prob -= *prob >> kNumMoveBits
		d.Code -= bound
		d.Range -= bound
		symbol = 1
	}

	// Normalize
	if d.Range < kTopValue {
		if d.bi >= len(d.b) {
			d.bi = 0
			d.b, d.btmp = d.btmp, d.b
			d.bSwapped = true
		}

		d.Range <<= 8
		d.Code = (d.Code << 8) | uint32(d.b[d.bi])
		d.bi++
	}

	return symbol
}

func (d *rangeDecoder) DecodeDirectBits(numBits int) uint32 {
	var res uint32

	for ; numBits > 0; numBits-- {
		d.Range >>= 1
		d.Code -= d.Range
		t := 0 - (d.Code >> 31)
		d.Code += d.Range & t

		if d.Code == d.Range {
			d.Corrupted = true
		}

		// Normalize
		if d.Range < kTopValue {
			if d.bi >= len(d.b) {
				d.bi = 0
				d.b, d.btmp = d.btmp, d.b
				d.bSwapped = true
			}

			d.Range <<= 8
			d.Code = (d.Code << 8) | uint32(d.b[d.bi])
			d.bi++
		}

		res <<= 1
		res += t + 1
	}

	return res
}
