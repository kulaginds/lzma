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
	rang := d.Range
	code := d.Code
	bound := (rang >> kNumBitModelTotalBits) * uint32(v)

	var symbol uint32

	if code < bound {
		v += ((1 << kNumBitModelTotalBits) - v) >> kNumMoveBits
		rang = bound
		symbol = 0
	} else {
		v -= v >> kNumMoveBits
		code -= bound
		rang -= bound
		symbol = 1
	}

	// Normalize
	if rang < kTopValue {
		b, err := d.inStream.ReadByte()
		if err != nil {
			return 0, err
		}

		rang <<= 8
		code = (code << 8) | uint32(b)
	}

	*prob = v
	d.Range = rang
	d.Code = code

	return symbol, nil
}

func (d *rangeDecoder) DecodeDirectBits(numBits int) (uint32, error) {
	var res uint32
	rang := d.Range
	code := d.Code

	for ; numBits > 0; numBits-- {
		rang >>= 1
		code -= rang
		t := 0 - (code >> 31)
		code += rang & t

		if code == rang {
			d.Corrupted = true
		}

		res <<= 1
		res += t + 1

		// Normalize
		if rang < kTopValue {
			b, err := d.inStream.ReadByte()
			if err != nil {
				return 0, err
			}

			rang <<= 8
			code = (code << 8) | uint32(b)
		}
	}

	d.Range = rang
	d.Code = code

	return res, nil
}
