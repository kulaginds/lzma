package lzma

import (
	"fmt"
	"io"
)

type Reader1 struct {
	inStream io.Reader

	unpackSizeDefined bool
	markerIsMandatory bool

	lc, pb, lp uint8

	dictSize   uint32
	unpackSize uint64

	outWindow      *windowWithPending
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

	rep0, rep1, rep2, rep3 uint32

	state    uint32
	posState uint32
	bit      uint32
	length   uint32
	dist     uint32
	isError  bool
}

func NewReader1(inStream io.Reader) (*Reader1, error) {
	r := &Reader1{
		inStream: inStream,

		rangeDec: newRangeDecoder(inStream),

		isMatch:    make([]uint16, kNumStates<<kNumPosBitsMax),
		isRep:      make([]uint16, kNumStates),
		isRepG0:    make([]uint16, kNumStates),
		isRepG1:    make([]uint16, kNumStates),
		isRepG2:    make([]uint16, kNumStates),
		isRep0Long: make([]uint16, kNumStates<<kNumPosBitsMax),
	}
	r.lenDecoder = newLenDecoder(r.rangeDec)
	r.repLenDecoder = newLenDecoder(r.rangeDec)

	return r, r.initialize()
}

func (r *Reader1) initialize() error {
	header := make([]byte, lzmaHeaderLen)

	n, err := r.inStream.Read(header)
	if err != nil {
		return err
	}

	if n != lzmaHeaderLen {
		return ErrCorrupted
	}

	err = r.decodeProperties(header)
	if err != nil {
		return fmt.Errorf("decodeProperties: %w", err)
	}

	err = r.decodeUnpackSize(header[5:])
	if err != nil {
		return fmt.Errorf("decode unpack size: %w", err)
	}

	r.outWindow = newWindowWithPending(r.dictSize)
	r.litProbs = make([]uint16, uint32(0x300)<<(r.lc+r.lp))

	var initialized bool

	initialized, err = r.rangeDec.Init()
	if err != nil {
		return fmt.Errorf("rangeDec.Init: %w", err)
	}

	if !initialized {
		return ErrResultError
	}

	r.initLiterals()
	r.initDist()
	initProbs(r.isMatch)
	initProbs(r.isRep)
	initProbs(r.isRepG0)
	initProbs(r.isRepG1)
	initProbs(r.isRepG2)
	initProbs(r.isRep0Long)
	r.lenDecoder.Init()
	r.repLenDecoder.Init()

	return nil
}

func (r *Reader1) initDist() {
	r.posSlotDecoder = make([]*bitTreeDecoder, kNumLenToPosStates)

	for i := 0; i < kNumLenToPosStates; i++ {
		r.posSlotDecoder[i] = newBitTreeDecoder(r.rangeDec, 6)
		r.posSlotDecoder[i].Init()
	}

	r.alignDecoder = newBitTreeDecoder(r.rangeDec, kNumAlignBits)
	r.alignDecoder.Init()

	r.posDecoders = make([]uint16, 1+kNumFullDistances-kEndPosModelIndex)
	initProbs(r.posDecoders)
}

func (r *Reader1) initLiterals() {
	num := uint32(0x300) << (r.lc + r.lp)

	for i := uint32(0); i < num; i++ {
		r.litProbs[i] = ProbInitVal
	}
}

func (r *Reader1) decodeUnpackSize(header []byte) error {
	var b byte

	for i := 0; i < 8; i++ {
		b = header[i]
		if b != 0xFF {
			r.unpackSizeDefined = true
		}

		r.unpackSize |= uint64(b) << (8 * i)
	}

	r.markerIsMandatory = !r.unpackSizeDefined

	return nil
}

func (r *Reader1) decodeProperties(properties []byte) error {
	d := properties[0]
	if d >= (9 * 5 * 5) {
		return ErrIncorrectProperties
	}

	properties = properties[1:]

	r.lc = d % 9
	d /= 9
	r.pb = d / 5
	r.lp = d % 5

	dictSizeInProperties := uint32(0)
	for i := 0; i < 4; i++ {
		dictSizeInProperties |= uint32(properties[i]) << (8 * i)
	}

	r.dictSize = dictSizeInProperties

	if r.dictSize < lzmaDicMin {
		r.dictSize = lzmaDicMin
	}

	if r.dictSize > lzmaDicMax {
		return ErrDictOutOfRange
	}

	return nil
}

func (r *Reader1) Read(p []byte) (n int, err error) {
	if r.unpackSizeDefined && r.unpackSize == 0 && !r.markerIsMandatory && r.rangeDec.IsFinishedOK() {
		return 0, io.EOF
	}

	if r.outWindow.HasPending() {
		n, err = r.outWindow.ReadPending(p)
		if err != nil {
			return n, err
		}

		if n == len(p) {
			return
		}

		p = p[n:]
	}

	targetUnpackSize := r.unpackSize - uint64(len(p))
	if r.unpackSize < uint64(len(p)) {
		targetUnpackSize = 0
	}

	for {
		if r.unpackSize < targetUnpackSize {
			break
		}

		if r.unpackSizeDefined && r.unpackSize == 0 && !r.markerIsMandatory {
			if r.rangeDec.IsFinishedOK() {
				err = io.EOF

				break
			}
		}

		err = r.rangeDec.WarmUp()
		if err != nil {
			return 0, err
		}

		r.posState = r.outWindow.TotalPos & ((1 << r.pb) - 1)

		r.bit = r.rangeDec.DecodeBit(&r.isMatch[(r.state<<kNumPosBitsMax)+r.posState])
		if r.bit == 0 {
			if r.unpackSizeDefined && r.unpackSize == 0 {
				return 0, ErrResultError
			}

			err = r.DecodeLiteral(r.state, r.rep0)
			if err != nil {
				return 0, fmt.Errorf("decode literal: %w", err)
			}

			r.state = stateUpdateLiteral(r.state)
			r.unpackSize--
			continue
		}

		r.bit = r.rangeDec.DecodeBit(&r.isRep[r.state])

		if r.bit != 0 {
			if r.unpackSizeDefined && r.unpackSize == 0 {
				return 0, ErrResultError
			}

			if r.outWindow.IsEmpty() {
				return 0, ErrResultError
			}

			r.bit = r.rangeDec.DecodeBit(&r.isRepG0[r.state])

			if r.bit == 0 {
				r.bit = r.rangeDec.DecodeBit(&r.isRep0Long[(r.state<<kNumPosBitsMax)+r.posState])

				if r.bit == 0 {
					r.state = stateUpdateShortRep(r.state)
					r.outWindow.PutByte(r.outWindow.GetByte(r.rep0 + 1))
					r.unpackSize--

					continue
				}
			} else {
				r.bit = r.rangeDec.DecodeBit(&r.isRepG1[r.state])

				if r.bit == 0 {
					r.dist = r.rep1
				} else {
					r.bit = r.rangeDec.DecodeBit(&r.isRepG2[r.state])
					if r.bit == 0 {
						r.dist = r.rep2
					} else {
						r.dist = r.rep3
						r.rep3 = r.rep2
					}

					r.rep2 = r.rep1
				}

				r.rep1 = r.rep0
				r.rep0 = r.dist
			}

			r.length = r.repLenDecoder.Decode(r.posState)

			r.state = stateUpdateRep(r.state)
		} else {
			r.rep3 = r.rep2
			r.rep2 = r.rep1
			r.rep1 = r.rep0

			r.length = r.lenDecoder.Decode(r.posState)

			r.state = stateUpdateMatch(r.state)
			r.rep0, err = r.DecodeDistance(r.length)
			if err != nil {
				return 0, fmt.Errorf("decode distance: %w", err)
			}

			if r.rep0 == 0xFFFFFFFF {
				if r.rangeDec.IsFinishedOK() {
					err = io.EOF

					break
				} else {
					return 0, ErrResultError
				}
			}

			if r.unpackSizeDefined && r.unpackSize == 0 {
				return 0, ErrResultError
			}

			if r.rep0 >= r.dictSize || !r.outWindow.CheckDistance(r.rep0) {
				return 0, ErrResultError
			}
		}

		r.length += kMatchMinLen
		if r.unpackSizeDefined && uint32(r.unpackSize) < r.length {
			r.length = uint32(r.unpackSize)
			r.isError = true
		}

		r.outWindow.CopyMatch(r.rep0+1, r.length)

		r.unpackSize -= uint64(r.length)

		if r.isError {
			return 0, ErrResultError
		}
	}

	if r.outWindow.HasPending() {
		oldN := n
		oldErr := err

		n, err = r.outWindow.ReadPending(p)
		n += oldN
		if err != nil {
			return n, err
		}

		err = oldErr
	}

	return
}

func (r *Reader1) DecodeLiteral(state uint32, rep0 uint32) error {
	prevByte := uint32(0)
	if !r.outWindow.IsEmpty() {
		prevByte = uint32(r.outWindow.GetByte(1))
	}

	symbol := uint32(1)
	litState := ((r.outWindow.TotalPos & ((1 << r.lp) - 1)) << r.lc) + (prevByte >> (8 - r.lc))
	probs := r.litProbs[(uint32(0x300) * litState):]

	if state >= 7 {
		matchByte := r.outWindow.GetByte(rep0 + 1)

		var bit uint32

		for symbol < 0x100 {
			matchBit := uint32((matchByte >> 7) & 1)
			matchByte <<= 1

			bit = r.rangeDec.DecodeBit(&probs[((1+matchBit)<<8)+symbol])

			symbol = (symbol << 1) | bit
			if matchBit != bit {
				break
			}
		}
	}

	for symbol < 0x100 {
		symbol = (symbol << 1) | r.rangeDec.DecodeBit(&probs[symbol])
	}

	r.outWindow.PutByte(byte(symbol - 0x100))

	return nil
}

func (r *Reader1) DecodeDistance(len uint32) (uint32, error) {
	lenState := len
	if lenState > (kNumLenToPosStates - 1) {
		lenState = kNumLenToPosStates - 1
	}

	posSlot := r.posSlotDecoder[lenState].Decode()

	if posSlot < 4 {
		return posSlot, nil
	}

	numDirectBits := (posSlot >> 1) - 1
	dist := (2 | (posSlot & 1)) << numDirectBits

	if posSlot < kEndPosModelIndex {
		dist += BitTreeReverseDecode(r.posDecoders[dist-posSlot:], int(numDirectBits), r.rangeDec)
	} else {
		dist += r.rangeDec.DecodeDirectBits(int(numDirectBits-kNumAlignBits)) << kNumAlignBits
		dist += r.alignDecoder.ReverseDecode()
	}

	return dist, nil
}
