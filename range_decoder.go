package lzma

import (
	"io"
)

type rangeDecoder struct {
	inStream io.ByteReader

	Range     uint32
	Code      uint32
	Corrupted bool
}

func newRangeDecoder(inStream io.ByteReader) *rangeDecoder {
	return &rangeDecoder{
		inStream: inStream,

		Range: 0xFFFFFFFF,
	}
}

func (d *rangeDecoder) IsFinishedOK() bool {
	return d.Code == 0
}

func (d *rangeDecoder) Init() error {
	b, err := d.inStream.ReadByte()
	if err != nil {
		return err
	}
	if b != 0 {
		return ErrResultError
	}

	for i := 0; i < 4; i++ {
		b, err = d.inStream.ReadByte()
		if err != nil {
			return err
		}

		d.Code = (d.Code << 8) | uint32(b)
	}

	return nil
}

func (d *rangeDecoder) Reopen(inStream io.ByteReader) error {
	d.inStream = inStream
	d.Corrupted = false
	d.Range = 0xFFFFFFFF
	d.Code = 0

	return d.Init()
}

func (d *rangeDecoder) DecodeBit(prob *prob) (uint32, error) {
	v := *prob
	bound := (d.Range >> kNumBitModelTotalBits) * uint32(v)

	var symbol uint32

	if d.Code < bound {
		v += ((1 << kNumBitModelTotalBits) - v) >> kNumMoveBits
		d.Range = bound
		symbol = 0
	} else {
		v -= v >> kNumMoveBits
		d.Code -= bound
		d.Range -= bound
		symbol = 1
	}

	*prob = v

	// Normalize
	if d.Range < kTopValue {
		b, err := d.inStream.ReadByte()
		if err != nil {
			return 0, err
		}

		d.Range <<= 8
		d.Code = (d.Code << 8) | uint32(b)
	}

	return symbol, nil
}

func (d *rangeDecoder) DecodeDirectBits(numBits int) (uint32, error) {
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
			b, err := d.inStream.ReadByte()
			if err != nil {
				return 0, err
			}

			d.Range <<= 8
			d.Code = (d.Code << 8) | uint32(b)
		}

		res <<= 1
		res += t + 1
	}

	return res, nil
}
