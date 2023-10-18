package lzma

import "fmt"

const (
	kNumPosBitsMax = 4

	kNumStates          = 12
	kNumLenToPosStates  = 4
	kNumAlignBits       = 4
	kStartPosModelIndex = 4
	kEndPosModelIndex   = 14
	kNumFullDistances   = 1 << (kEndPosModelIndex >> 1)
	kMatchMinLen        = 2
)

type lenDecoder struct {
	rc *rangeDecoder

	choice  uint16
	choice2 uint16

	lowCoder  []*bitTreeDecoder
	midCoder  []*bitTreeDecoder
	highCoder *bitTreeDecoder
}

func newLenDecoder(rc *rangeDecoder) *lenDecoder {
	return &lenDecoder{
		rc: rc,

		choice:  ProbInitVal,
		choice2: ProbInitVal,

		lowCoder:  make([]*bitTreeDecoder, 1<<kNumPosBitsMax),
		midCoder:  make([]*bitTreeDecoder, 1<<kNumPosBitsMax),
		highCoder: newBitTreeDecoder(rc, 8),
	}
}

func (d *lenDecoder) Init() {
	for i := 0; i < len(d.lowCoder); i++ {
		d.lowCoder[i] = newBitTreeDecoder(d.rc, 3)
		d.lowCoder[i].Init()

		d.midCoder[i] = newBitTreeDecoder(d.rc, 3)
		d.midCoder[i].Init()
	}

	d.highCoder.Init()
}

func (d *lenDecoder) Decode(posState uint32) (uint32, error) {
	bit, err := d.rc.DecodeBit(&d.choice)
	if err != nil {
		return 0, fmt.Errorf("decode bit: %w", err)
	}

	if bit == 0 {
		return d.lowCoder[posState].Decode()
	}

	bit, err = d.rc.DecodeBit(&d.choice2)
	if bit == 0 {
		bit, err = d.midCoder[posState].Decode()

		return 8 + bit, err
	}

	bit, err = d.highCoder.Decode()

	return 16 + bit, err
}
