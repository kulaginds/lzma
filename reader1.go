package lzma

import (
	"errors"
	"fmt"
	"io"
)

type Reader1 struct {
	rangeDec  *rangeDecoder
	outWindow *window

	s             *state
	isEndOfStream bool

	chunkCounter int64
	opCounter    int64
}

func NewReader1(inStream io.Reader) (*Reader1, error) {
	r := &Reader1{
		rangeDec: newRangeDecoder(inStream),
	}

	return r, r.initializeFull(inStream)
}

func NewReader1ForSevenZip(inStream io.Reader, props []byte, unpackSize uint64) (*Reader1, error) {
	lc, pb, lp, err := DecodeProp(props[0])
	if err != nil {
		return nil, err
	}

	dictSize, err := DecodeDictSize(props[1:5])
	if err != nil {
		return nil, err
	}

	r := &Reader1{
		rangeDec:  newRangeDecoder(inStream),
		outWindow: newWindow(dictSize),
	}

	return r, r.initialize(lc, pb, lp, unpackSize)
}

func NewReader1ForReader2(inStream io.Reader, prop byte, unpackSize uint64, outWindow *window) (*Reader1, error) {
	lc, pb, lp, err := DecodeProp(prop)
	if err != nil {
		return nil, err
	}

	r := &Reader1{
		outWindow: outWindow,
		rangeDec:  newRangeDecoder(inStream),
	}

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

	lc, pb, lp, err := DecodeProp(header[0])
	if err != nil {
		return fmt.Errorf("decode prop: %w", err)
	}

	dictSize, err := DecodeDictSize(header[1:5])
	if err != nil {
		return fmt.Errorf("decode dict size: %w", err)
	}

	r.outWindow = newWindow(dictSize)

	unpackSize := DecodeUnpackSize(header[5:])

	return r.initialize(lc, pb, lp, unpackSize)
}

func (r *Reader1) initialize(lc, pb, lp uint8, unpackSize uint64) error {
	r.s = newState(lc, pb, lp)
	r.s.SetUnpackSize(unpackSize)

	err := r.rangeDec.Init()
	if err != nil {
		return fmt.Errorf("rangeDec.Reset: %w", err)
	}

	return nil
}

func (r *Reader1) Reset() {
	r.s.Reset()
	r.isEndOfStream = false
}

func (r *Reader1) Reopen(inStream io.Reader, unpackSize uint64) error {
	r.isEndOfStream = false
	r.s.SetUnpackSize(unpackSize)

	err := r.rangeDec.Reopen(inStream)
	if err != nil {
		return err
	}

	return nil
}

func DecodeUnpackSize(header []byte) uint64 {
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

func DecodeDictSize(properties []byte) (uint32, error) {
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

func DecodeProp(d byte) (uint8, uint8, uint8, error) {
	if d >= (9 * 5 * 5) {
		return 0, 0, 0, ErrIncorrectProperties
	}

	lc := d % 9
	d /= 9
	pb := d / 5
	lp := d % 5

	return lc, pb, lp, nil
}

func (r *Reader1) Read(p []byte) (n int, err error) {
	var k int

	for {
		if r.outWindow.HasPending() {
			k, err = r.outWindow.ReadPending(p[n:])
			n += k
			if err != nil {
				return n, err
			}

			if n >= len(p) {
				return
			}
		}

		if r.isEndOfStream {
			err = io.EOF

			return
		}

		err = r.decompress()
		if errors.Is(err, io.EOF) {
			r.isEndOfStream = true
			err = nil
		}
		if err != nil {
			return
		}
	}
}

func (r *Reader1) decompress() (err error) {
	for r.outWindow.Available() >= maxMatchLen {
		err = r.rangeDec.WarmUp()
		if err != nil {
			return err
		}

		err = r.decodeOperation()
		if errors.Is(err, io.EOF) {
			if !r.rangeDec.IsFinishedOK() {
				err = ErrResultError
			}

			break
		}
		if err != nil {
			return err
		}
	}

	return
}

func (r *Reader1) printOp(op string) {
	if r.chunkCounter == 1 {
		fmt.Println(r.opCounter, op, r.rangeDec.Code, r.rangeDec.Range)
	}
}

func (r *Reader1) decodeOperation() error {
	var err error

	s := r.s

	if s.unpackSizeDefined && s.bytesLeft == 0 && !s.markerIsMandatory {
		if r.rangeDec.IsFinishedOK() {
			return io.EOF
		}
	}

	s.posState = r.outWindow.TotalPos & s.posMask
	r.opCounter++

	if r.chunkCounter == 1 && r.opCounter == 15198 {
		a := 5
		_ = a
	}

	if r.rangeDec.DecodeBit(&s.isMatch[(s.state<<kNumPosBitsMax)+s.posState]) == 0 { // literal
		if s.unpackSizeDefined && s.bytesLeft == 0 {
			return ErrResultError
		}

		r.printOp("l")
		err = r.DecodeLiteral(s.state, s.rep0)
		if err != nil {
			return fmt.Errorf("decode literal: %w", err)
		}

		s.state = stateUpdateLiteral(s.state)
		s.bytesLeft--

		return nil
	}

	length := uint32(0)

	if r.rangeDec.DecodeBit(&s.isRep[s.state]) == 0 { // simple match
		s.rep3, s.rep2, s.rep1 = s.rep2, s.rep1, s.rep0

		r.printOp("m")
		length = s.lenDecoder.Decode(r.rangeDec, s.posState)
		s.state = stateUpdateMatch(s.state)
		s.rep0 = r.DecodeDistance(length)

		if s.rep0 == 0xFFFFFFFF {
			if r.rangeDec.IsFinishedOK() {
				if s.unpackSizeDefined && s.bytesLeft > 0 && !s.markerIsMandatory {
					return ErrResultError
				}

				return io.EOF
			} else {
				return ErrResultError
			}
		}

		if s.unpackSizeDefined && s.bytesLeft == 0 {
			return ErrResultError
		}

		if s.rep0 >= r.outWindow.size || !r.outWindow.CheckDistance(s.rep0) {
			return ErrResultError
		}
	} else { // rep match
		if s.unpackSizeDefined && s.bytesLeft == 0 {
			return ErrResultError
		}

		if r.outWindow.IsEmpty() {
			return ErrResultError
		}

		if r.rangeDec.DecodeBit(&s.isRepG0[s.state]) == 0 { // short rep match
			r.printOp("s")
			if r.rangeDec.DecodeBit(&s.isRep0Long[(s.state<<kNumPosBitsMax)+s.posState]) == 0 {
				s.state = stateUpdateShortRep(s.state)
				r.outWindow.PutByte(r.outWindow.GetByte(s.rep0 + 1))
				s.bytesLeft--

				return nil
			}
		} else { // rep match
			dist := uint32(0)
			if r.rangeDec.DecodeBit(&s.isRepG1[s.state]) == 0 {
				dist = s.rep1
			} else {
				if r.rangeDec.DecodeBit(&s.isRepG2[s.state]) == 0 {
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

		r.printOp("r")
		length = r.s.repLenDecoder.Decode(r.rangeDec, s.posState)
		s.state = stateUpdateRep(s.state)
	}

	length += kMatchMinLen
	isError := false
	if s.unpackSizeDefined && uint32(s.bytesLeft) < length {
		length = uint32(s.bytesLeft)
		isError = true
	}

	r.outWindow.CopyMatch(s.rep0+1, length)

	s.bytesLeft -= uint64(length)

	if isError {
		return ErrResultError
	}

	return nil
}

func (r *Reader1) DecodeLiteral(state uint32, rep0 uint32) error {
	s := r.s
	prevByte := uint32(0)
	if !r.outWindow.IsEmpty() {
		prevByte = uint32(r.outWindow.GetByte(1))
	}

	symbol := uint32(1)
	litState := ((r.outWindow.TotalPos & ((1 << s.lp) - 1)) << s.lc) + (prevByte >> (8 - s.lc))
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

func (r *Reader1) DecodeDistance(len uint32) uint32 {
	lenState := len
	if lenState > (kNumLenToPosStates - 1) {
		lenState = kNumLenToPosStates - 1
	}

	s := r.s
	posSlot := s.posSlotDecoder[lenState].Decode(r.rangeDec)

	if posSlot < 4 {
		return posSlot
	}

	numDirectBits := (posSlot >> 1) - 1
	dist := (2 | (posSlot & 1)) << numDirectBits

	if posSlot < kEndPosModelIndex {
		dist += BitTreeReverseDecode(s.posDecoders[dist-posSlot:], int(numDirectBits), r.rangeDec)
	} else {
		dist += r.rangeDec.DecodeDirectBits(int(numDirectBits-kNumAlignBits)) << kNumAlignBits
		dist += s.alignDecoder.ReverseDecode(r.rangeDec)
	}

	return dist
}
