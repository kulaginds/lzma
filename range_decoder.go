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

	b, btmp      []byte
	bi           int
	bSwapped     bool
	bInitialized bool

	readByte []byte
	readErr  error
}

func newRangeDecoder(inStream io.Reader) *rangeDecoder {
	return &rangeDecoder{
		inStream: inStream,

		Range: 0xFFFFFFFF,

		b:        make([]byte, lzmaRequiredInputMax+1),
		btmp:     make([]byte, lzmaRequiredInputMax+1),
		bSwapped: true,

		readByte: make([]byte, 1),
	}
}

func (d *rangeDecoder) IsFinishedOK() bool {
	return d.Code == 0
}

func (d *rangeDecoder) Init() error {
	header := make([]byte, rangeDecoderHeaderLen)

	n, err := d.inStream.Read(header)
	if err != nil {
		return fmt.Errorf("inStream.Read: %w", err)
	}

	if n != rangeDecoderHeaderLen {
		return ErrCorrupted
	}

	return d.init(header)
}

func (d *rangeDecoder) init(header []byte) error {
	b := header[0]
	header = header[1:]

	for i := 0; i < len(header); i++ {
		d.Code = (d.Code << 8) | uint32(header[i])
	}

	if b != 0 || d.Code == d.Range {
		d.Corrupted = true
	}

	if b != 0 {
		return ErrResultError
	}

	return nil
}

func (d *rangeDecoder) Reopen(inStream io.Reader) error {
	d.inStream = inStream
	d.Corrupted = false
	d.Range = 0xFFFFFFFF
	d.Code = 0

	return d.Init()

	//header := make([]byte, rangeDecoderHeaderLen)
	//
	//var (
	//	noBufferedInput bool
	//	readFrom        int
	//)
	//
	//for i := 0; i < len(header); i++ {
	//	if d.bi >= len(d.b) && !d.bSwapped {
	//		d.bi = 0
	//		d.b, d.btmp = d.btmp, d.b
	//		d.bSwapped = true
	//	}
	//
	//	if d.bi >= len(d.b) {
	//		noBufferedInput = true
	//		readFrom = i
	//
	//		break
	//	}
	//
	//	header[i] = d.b[d.bi]
	//	d.bi++
	//}
	//
	//if noBufferedInput {
	//	_, err := d.inStream.Read(header[readFrom:])
	//	if err != nil {
	//		return err
	//	}
	//
	//	err = d.init(header)
	//	if err != nil {
	//		return err
	//	}
	//}

	//return nil
}

func (d *rangeDecoder) WarmUp() error {
	//var (
	//	n   int
	//	err error
	//)
	//
	//if !d.bInitialized {
	//	n, err = d.inStream.Read(d.b)
	//	if err != nil && !errors.Is(err, io.EOF) {
	//		return err
	//	}
	//
	//	d.b = d.b[:n]
	//	d.bInitialized = true
	//}
	//
	//if !d.bSwapped {
	//	return nil
	//}
	//
	//d.btmp = d.btmp[:cap(d.btmp)]
	//n, err = d.inStream.Read(d.btmp)
	//if err != nil && !errors.Is(err, io.EOF) {
	//	return err
	//}
	//
	//d.btmp = d.btmp[:n]
	//d.bSwapped = false

	return d.readErr
}

func (d *rangeDecoder) DecodeBit(prob *prob) uint32 {
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
		//if d.bi >= len(d.b) {
		//	d.bi = 0
		//	d.b, d.btmp = d.btmp, d.b
		//	d.bSwapped = true
		//}
		_, d.readErr = d.inStream.Read(d.readByte)

		d.Range <<= 8
		d.Code = (d.Code << 8) | uint32(d.readByte[0])
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
			//if d.bi >= len(d.b) {
			//	d.bi = 0
			//	d.b, d.btmp = d.btmp, d.b
			//	d.bSwapped = true
			//}
			_, d.readErr = d.inStream.Read(d.readByte)

			d.Range <<= 8
			d.Code = (d.Code << 8) | uint32(d.readByte[0])
			d.bi++
		}

		res <<= 1
		res += t + 1
	}

	return res
}
