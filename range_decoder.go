package lzma

import (
	"fmt"
	"io"
)

type rangeDecoder struct {
	inStream io.Reader

	Range     uint32
	Code      uint32
	Corrupted bool

	b []byte
}

func newRangeDecoder(inStream io.Reader) *rangeDecoder {
	return &rangeDecoder{
		inStream: inStream,

		Range: 0xFFFFFFFF,

		b: make([]byte, 1),
	}
}

func (d *rangeDecoder) IsFinishedOK() bool {
	return d.Code == 0
}

const rangeDecoderHeaderLen = 5

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

const kTopValue = uint32(1) << 24

func (d *rangeDecoder) Normalize() error {
	if d.Range < kTopValue {
		_, err := d.inStream.Read(d.b)
		if err != nil {
			return fmt.Errorf("read byte: %w", err)
		}

		d.Range <<= 8
		d.Code = (d.Code << 8) | uint32(d.b[0])
	}

	return nil
}

func (d *rangeDecoder) DecodeBit(prob *uint16) (uint32, error) {
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

	err := d.Normalize()
	if err != nil {
		return 0, fmt.Errorf("normalize: %w", err)
	}

	return symbol, nil
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

		d.Normalize()
		res <<= 1
		res += t + 1
	}

	return res
}
