package lzma

import (
	"fmt"
	"io"
)

type Reader1 struct {
	outWindow *windowWithPending
	rangeDec  *rangeDecoder

	s *state
}

func NewReader1(inStream io.Reader) (*Reader1, error) {
	r := &Reader1{
		rangeDec: newRangeDecoder(inStream),
	}

	return r, r.initializeFull(inStream)
}

func NewReader1WithOptions(inStream io.Reader, prop byte, unpackSize uint64, outWindow *windowWithPending) (*Reader1, error) {
	r := &Reader1{
		outWindow: outWindow,
		rangeDec:  newRangeDecoder(inStream),
	}

	lc, pb, lp := decodeProp(prop)

	return r, r.initialize(lc, pb, lp, unpackSize)
}

func (r *Reader1) initializeFull(inStream io.Reader) error {
	header := make([]byte, lzmaHeaderLen)

	n, err := inStream.Read(header)
	if err != nil {
		return err
	}

	if n != lzmaHeaderLen {
		return ErrCorrupted
	}

	if header[0] >= (9 * 5 * 5) {
		return ErrIncorrectProperties
	}

	lc, pb, lp := decodeProp(header[0])

	dictSize, err := r.decodeDictSize(header[1:5])
	if err != nil {
		return fmt.Errorf("decode dict size: %w", err)
	}

	r.outWindow = newWindowWithPending(dictSize)

	unpackSize := r.decodeUnpackSize(header[5:])

	return r.initialize(lc, pb, lp, unpackSize)
}

func (r *Reader1) initialize(lc, pb, lp uint8, unpackSize uint64) error {
	r.s = newState(lc, pb, lp)
	r.s.SetUnpackSize(unpackSize)

	initialized, err := r.rangeDec.Init()
	if err != nil {
		return fmt.Errorf("rangeDec.Reset: %w", err)
	}

	if !initialized {
		return ErrResultError
	}

	return nil
}

func (r *Reader1) Reset() {
	r.s.Reset()
}

func (r *Reader1) Reopen(inStream io.Reader, unpackSize uint64) error {
	r.rangeDec = newRangeDecoder(inStream)
	r.s.SetUnpackSize(unpackSize)

	initialized, err := r.rangeDec.Init()
	if err != nil {
		return err
	}

	if !initialized {
		return ErrResultError
	}

	return nil
}

func (r *Reader1) decodeUnpackSize(header []byte) uint64 {
	var (
		b          byte
		unpackSize uint64
	)

	for i := 0; i < 8; i++ {
		b = header[i]

		unpackSize |= uint64(b) << (8 * i)
	}

	return unpackSize
}

func (r *Reader1) decodeDictSize(properties []byte) (uint32, error) {
	dictSize := uint32(0)
	for i := 0; i < 4; i++ {
		dictSize |= uint32(properties[i]) << (8 * i)
	}

	if dictSize < lzmaDicMin {
		dictSize = lzmaDicMin
	}

	if dictSize > lzmaDicMax {
		return 0, ErrDictOutOfRange
	}

	return dictSize, nil
}

func decodeProp(d byte) (uint8, uint8, uint8) {
	lc := d % 9
	d /= 9
	pb := d / 5
	lp := d % 5

	return lc, pb, lp
}

func (r *Reader1) Read(p []byte) (n int, err error) {
	s := r.s

	if r.outWindow.HasPending() {
		n, err = r.outWindow.ReadPending(p)
		if err != nil {
			return n, err
		}

		if n >= len(p) {
			return
		}

		p = p[n:]
	}

	if s.unpackSizeDefined && s.unpackSize == 0 && !s.markerIsMandatory && r.rangeDec.IsFinishedOK() {
		return 0, io.EOF
	}

	targetUnpackSize := s.unpackSize - uint64(len(p))
	if s.unpackSize < uint64(len(p)) {
		targetUnpackSize = 0
	}

	bit := uint32(0)
	length := uint32(0)
	dist := uint32(0)
	isError := false

	for {
		if s.unpackSize <= targetUnpackSize {
			break
		}

		if s.unpackSizeDefined && s.unpackSize == 0 && !s.markerIsMandatory {
			if r.rangeDec.IsFinishedOK() {
				err = io.EOF

				break
			}
		}

		err = r.rangeDec.WarmUp()
		if err != nil {
			return 0, err
		}

		s.posState = r.outWindow.pos & ((1 << s.pb) - 1)

		bit = r.rangeDec.DecodeBit(&s.isMatch[(s.state<<kNumPosBitsMax)+s.posState])
		if bit == 0 {
			if s.unpackSizeDefined && s.unpackSize == 0 {
				return 0, ErrResultError
			}

			err = r.DecodeLiteral(s.state, s.rep0)
			if err != nil {
				return 0, fmt.Errorf("decode literal: %w", err)
			}

			s.state = stateUpdateLiteral(s.state)
			s.unpackSize--
			continue
		}

		bit = r.rangeDec.DecodeBit(&s.isRep[s.state])

		if bit != 0 {
			if s.unpackSizeDefined && s.unpackSize == 0 {
				return 0, ErrResultError
			}

			if r.outWindow.IsEmpty() {
				return 0, ErrResultError
			}

			bit = r.rangeDec.DecodeBit(&s.isRepG0[s.state])
			if bit == 0 {
				bit = r.rangeDec.DecodeBit(&s.isRep0Long[(s.state<<kNumPosBitsMax)+s.posState])

				if bit == 0 {
					s.state = stateUpdateShortRep(s.state)
					r.outWindow.PutByte(r.outWindow.GetByte(s.rep0 + 1))
					s.unpackSize--

					continue
				}
			} else {
				bit = r.rangeDec.DecodeBit(&s.isRepG1[s.state])
				if bit == 0 {
					dist = s.rep1
				} else {
					bit = r.rangeDec.DecodeBit(&s.isRepG2[s.state])
					if bit == 0 {
						dist = s.rep2
					} else {
						dist = s.rep3
						s.rep3 = s.rep2
					}

					s.rep2 = s.rep1
				}

				s.rep1 = s.rep0
				s.rep0 = dist
			}

			length = r.s.repLenDecoder.Decode(r.rangeDec, s.posState)

			s.state = stateUpdateRep(s.state)
		} else {
			s.rep3 = s.rep2
			s.rep2 = s.rep1
			s.rep1 = s.rep0

			length = s.lenDecoder.Decode(r.rangeDec, s.posState)

			s.state = stateUpdateMatch(s.state)
			s.rep0, err = r.DecodeDistance(length)
			if err != nil {
				return 0, fmt.Errorf("decode distance: %w", err)
			}

			if s.rep0 == 0xFFFFFFFF {
				if r.rangeDec.IsFinishedOK() {
					if s.unpackSizeDefined && s.unpackSize != 0 && !s.markerIsMandatory {
						return 0, ErrResultError
					}

					err = io.EOF

					break
				} else {
					return 0, ErrResultError
				}
			}

			if s.unpackSizeDefined && s.unpackSize == 0 {
				return 0, ErrResultError
			}

			if s.rep0 >= r.outWindow.size || !r.outWindow.CheckDistance(s.rep0) {
				return 0, ErrResultError
			}
		}

		length += kMatchMinLen
		if s.unpackSizeDefined && uint32(s.unpackSize) < length {
			length = uint32(s.unpackSize)
			isError = true
		}

		r.outWindow.CopyMatch(s.rep0+1, length)

		s.unpackSize -= uint64(length)

		if isError {
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

	s := r.s
	symbol := uint32(1)
	litState := ((r.outWindow.pos & ((1 << s.lp) - 1)) << s.lc) + (prevByte >> (8 - s.lc))
	probs := s.litProbs[(uint32(0x300) * litState):]

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

	s := r.s
	posSlot := s.posSlotDecoder[lenState].Decode(r.rangeDec)

	if posSlot < 4 {
		return posSlot, nil
	}

	numDirectBits := (posSlot >> 1) - 1
	dist := (2 | (posSlot & 1)) << numDirectBits

	if posSlot < kEndPosModelIndex {
		dist += BitTreeReverseDecode(s.posDecoders[dist-posSlot:], int(numDirectBits), r.rangeDec)
	} else {
		dist += r.rangeDec.DecodeDirectBits(int(numDirectBits-kNumAlignBits)) << kNumAlignBits
		dist += s.alignDecoder.ReverseDecode(r.rangeDec)
	}

	return dist, nil
}
