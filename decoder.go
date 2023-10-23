package lzma

import (
	"fmt"
	"io"
)

type Decoder struct {
	inStream  io.Reader
	outStream io.Writer

	unpackSizeDefined bool
	markerIsMandatory bool

	lc, pb, lp uint8

	dictSize   uint32
	unpackSize uint64

	outWindow      *window
	rangeDec       *rangeDecoder
	posSlotDecoder []*bitTreeDecoder
	alignDecoder   *bitTreeDecoder
	lenDecoder     *lenDecoder
	repLenDecoder  *lenDecoder

	litProbs    []uint16
	posDecoders []uint16

	isMatch    []uint16
	isRep      []uint16
	isRepG0    []uint16
	isRepG1    []uint16
	isRepG2    []uint16
	isRep0Long []uint16
}

func NewDecoder(inStream io.Reader, outStream io.Writer) *Decoder {
	return &Decoder{
		inStream:  inStream,
		outStream: outStream,

		isMatch:    make([]uint16, kNumStates<<kNumPosBitsMax),
		isRep:      make([]uint16, kNumStates),
		isRepG0:    make([]uint16, kNumStates),
		isRepG1:    make([]uint16, kNumStates),
		isRepG2:    make([]uint16, kNumStates),
		isRep0Long: make([]uint16, kNumStates<<kNumPosBitsMax),
	}
}

const lzmaHeaderLen = 13

func (dec *Decoder) Init() error {
	header := make([]byte, lzmaHeaderLen)

	n, err := dec.inStream.Read(header)
	if err != nil {
		return err
	}

	if n != lzmaHeaderLen {
		return ErrCorrupted
	}

	err = dec.decodeProperties(header)
	if err != nil {
		return fmt.Errorf("decodeProperties: %w", err)
	}

	err = dec.decodeUnpackSize(header[5:])
	if err != nil {
		return fmt.Errorf("decode unpack size: %w", err)
	}

	dec.outWindow = newWindow(dec.outStream, dec.dictSize)

	dec.litProbs = make([]uint16, uint32(0x300)<<(dec.lc+dec.lp))

	dec.rangeDec = newRangeDecoder(dec.inStream)

	var initialized bool

	initialized, err = dec.rangeDec.Init()
	if err != nil {
		return fmt.Errorf("rangeDec.Init: %w", err)
	}

	if !initialized {
		return ErrResultError
	}

	dec.initLiterals()
	dec.initDist()
	initProbs(dec.isMatch)
	initProbs(dec.isRep)
	initProbs(dec.isRepG0)
	initProbs(dec.isRepG1)
	initProbs(dec.isRepG2)
	initProbs(dec.isRep0Long)

	dec.lenDecoder = newLenDecoder(dec.rangeDec)
	dec.lenDecoder.Init()

	dec.repLenDecoder = newLenDecoder(dec.rangeDec)
	dec.repLenDecoder.Init()

	return nil
}

func (dec *Decoder) initDist() {
	dec.posSlotDecoder = make([]*bitTreeDecoder, kNumLenToPosStates)

	for i := 0; i < kNumLenToPosStates; i++ {
		dec.posSlotDecoder[i] = newBitTreeDecoder(dec.rangeDec, 6)
		dec.posSlotDecoder[i].Init()
	}

	dec.alignDecoder = newBitTreeDecoder(dec.rangeDec, kNumAlignBits)
	dec.alignDecoder.Init()

	dec.posDecoders = make([]uint16, 1+kNumFullDistances-kEndPosModelIndex)
	initProbs(dec.posDecoders)
}

const (
	kNumBitModelTotalBits = 11
	kNumMoveBits          = 5
	ProbInitVal           = (1 << kNumBitModelTotalBits) / 2
)

func (dec *Decoder) initLiterals() {
	num := uint32(0x300) << (dec.lc + dec.lp)

	for i := uint32(0); i < num; i++ {
		dec.litProbs[i] = ProbInitVal
	}
}

func (dec *Decoder) decodeUnpackSize(header []byte) error {
	var b byte

	for i := 0; i < 8; i++ {
		b = header[i]
		if b != 0xFF {
			dec.unpackSizeDefined = true
		}

		dec.unpackSize |= uint64(b) << (8 * i)
	}

	dec.markerIsMandatory = !dec.unpackSizeDefined

	return nil
}

const (
	lzmaDicMin = 1 << 12
	lzmaDicMax = 1<<32 - 1
)

func (dec *Decoder) decodeProperties(properties []byte) error {
	d := properties[0]
	if d >= (9 * 5 * 5) {
		return ErrIncorrectProperties
	}

	properties = properties[1:]

	dec.lc = d % 9
	d /= 9
	dec.pb = d / 5
	dec.lp = d % 5

	dictSizeInProperties := uint32(0)
	for i := 0; i < 4; i++ {
		dictSizeInProperties |= uint32(properties[i]) << (8 * i)
	}

	dec.dictSize = dictSizeInProperties

	if dec.dictSize < lzmaDicMin {
		dec.dictSize = lzmaDicMin
	}

	if dec.dictSize > lzmaDicMax {
		return ErrDictOutOfRange
	}

	return nil
}

func (dec *Decoder) Decode() error {
	var err error

	var (
		rep0, rep1, rep2, rep3 uint32

		state    uint32
		posState uint32
		bit      uint32
		length   uint32
		dist     uint32
		isError  bool
	)

	for {
		if dec.unpackSizeDefined && dec.unpackSize == 0 && !dec.markerIsMandatory {
			if dec.rangeDec.IsFinishedOK() {
				return nil
			}
		}

		posState = dec.outWindow.TotalPos & ((1 << dec.pb) - 1)

		bit, err = dec.rangeDec.DecodeBit(&dec.isMatch[(state<<kNumPosBitsMax)+posState])
		if err != nil {
			return fmt.Errorf("decode bit: %w", err)
		}
		if bit == 0 {
			if dec.unpackSizeDefined && dec.unpackSize == 0 {
				return ErrResultError
			}

			err = dec.DecodeLiteral(state, rep0)
			if err != nil {
				return fmt.Errorf("decode literal: %w", err)
			}

			state = UpdateState_Literal(state)
			dec.unpackSize--
			continue
		}

		bit, err = dec.rangeDec.DecodeBit(&dec.isRep[state])
		if err != nil {
			return fmt.Errorf("decode bit: %w", err)
		}

		if bit != 0 {
			if dec.unpackSizeDefined && dec.unpackSize == 0 {
				return ErrResultError
			}

			if dec.outWindow.IsEmpty() {
				return ErrResultError
			}

			bit, err = dec.rangeDec.DecodeBit(&dec.isRepG0[state])
			if err != nil {
				return fmt.Errorf("decode bit: %w", err)
			}

			if bit == 0 {
				bit, err = dec.rangeDec.DecodeBit(&dec.isRep0Long[(state<<kNumPosBitsMax)+posState])
				if err != nil {
					return fmt.Errorf("decode bit: %w", err)
				}

				if bit == 0 {
					state = UpdateState_ShortRep(state)
					err = dec.outWindow.PutByte(dec.outWindow.GetByte(rep0 + 1))
					if err != nil {
						return fmt.Errorf("put byte: %w", err)
					}

					dec.unpackSize--
					continue
				}
			} else {
				bit, err = dec.rangeDec.DecodeBit(&dec.isRepG1[state])
				if err != nil {
					return fmt.Errorf("decode bit: %w", err)
				}

				if bit == 0 {
					dist = rep1
				} else {
					bit, err = dec.rangeDec.DecodeBit(&dec.isRepG2[state])
					if err != nil {
						return fmt.Errorf("decode bit: %w", err)
					}

					if bit == 0 {
						dist = rep2
					} else {
						dist = rep3
						rep3 = rep2
					}

					rep2 = rep1
				}

				rep1 = rep0
				rep0 = dist
			}

			length, err = dec.repLenDecoder.Decode(posState)
			if err != nil {
				return fmt.Errorf("rep length Decoder decode: %w", err)
			}

			state = UpdateState_Rep(state)
		} else {
			rep3 = rep2
			rep2 = rep1
			rep1 = rep0

			length, err = dec.lenDecoder.Decode(posState)
			if err != nil {
				return fmt.Errorf("length Decoder decode: %w", err)
			}

			state = UpdateState_Match(state)
			rep0, err = dec.DecodeDistance(length)
			if err != nil {
				return fmt.Errorf("decode distance: %w", err)
			}

			if rep0 == 0xFFFFFFFF {
				if dec.rangeDec.IsFinishedOK() {
					return nil
				} else {
					return ErrResultError
				}
			}

			if dec.unpackSizeDefined && dec.unpackSize == 0 {
				return ErrResultError
			}

			if rep0 >= dec.dictSize || !dec.outWindow.CheckDistance(rep0) {
				return ErrResultError
			}
		}

		length += kMatchMinLen
		if dec.unpackSizeDefined && uint32(dec.unpackSize) < length {
			length = uint32(dec.unpackSize)
			isError = true
		}

		err = dec.outWindow.CopyMatch(rep0+1, length)
		if err != nil {
			return err
		}

		dec.unpackSize -= uint64(length)

		if isError {
			return ErrResultError
		}
	}
}

func (dec *Decoder) DecodeLiteral(state uint32, rep0 uint32) error {
	prevByte := uint32(0)
	if !dec.outWindow.IsEmpty() {
		prevByte = uint32(dec.outWindow.GetByte(1))
	}

	symbol := uint32(1)
	litState := ((dec.outWindow.TotalPos & ((1 << dec.lp) - 1)) << dec.lc) + (prevByte >> (8 - dec.lc))
	probs := dec.litProbs[(uint32(0x300) * litState):]

	if state >= 7 {
		matchByte := dec.outWindow.GetByte(rep0 + 1)

		for symbol < 0x100 {
			matchBit := uint32((matchByte >> 7) & 1)
			matchByte <<= 1

			bit, err := dec.rangeDec.DecodeBit(&probs[((1+matchBit)<<8)+symbol])
			if err != nil {
				return fmt.Errorf("decode bit: %w", err)
			}

			symbol = (symbol << 1) | bit
			if matchBit != bit {
				break
			}
		}
	}

	for symbol < 0x100 {
		bit, err := dec.rangeDec.DecodeBit(&probs[symbol])
		if err != nil {
			return fmt.Errorf("decode bit: %w", err)
		}

		symbol = (symbol << 1) | bit
	}

	err := dec.outWindow.PutByte(byte(symbol - 0x100))
	if err != nil {
		return fmt.Errorf("put byte: %w", err)
	}

	return nil
}

func (dec *Decoder) DecodeDistance(len uint32) (uint32, error) {
	lenState := len
	if lenState > (kNumLenToPosStates - 1) {
		lenState = kNumLenToPosStates - 1
	}

	posSlot, err := dec.posSlotDecoder[lenState].Decode()
	if err != nil {
		return 0, fmt.Errorf("pos slot Decoder decode: %w", err)
	}

	if posSlot < 4 {
		return posSlot, nil
	}

	numDirectBits := (posSlot >> 1) - 1
	dist := (2 | (posSlot & 1)) << numDirectBits

	if posSlot < kEndPosModelIndex {
		locDist, err := BitTreeReverseDecode(dec.posDecoders[dist-posSlot:], int(numDirectBits), dec.rangeDec)
		if err != nil {
			return 0, fmt.Errorf("bit tree reverse decode: %w", err)
		}

		dist += locDist
	} else {
		bits, err := dec.rangeDec.DecodeDirectBits(int(numDirectBits - kNumAlignBits))
		if err != nil {
			return 0, fmt.Errorf("decode direct bits: %w", err)
		}

		dist += bits << kNumAlignBits

		bits, err = dec.alignDecoder.ReverseDecode()
		if err != nil {
			return 0, fmt.Errorf("align reverse decode: %w", err)
		}

		dist += bits
	}

	return dist, nil
}

func Decode(inStream io.Reader, outStream io.Writer) error {
	d := NewDecoder(inStream, outStream)

	err := d.Init()
	if err != nil {
		return err
	}

	return d.Decode()
}
