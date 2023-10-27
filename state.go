package lzma

type state struct {
	unpackSize uint64

	unpackSizeDefined bool
	markerIsMandatory bool

	lc, pb, lp uint8

	posMask uint32

	posSlotDecoder []*bitTreeDecoder
	alignDecoder   *bitTreeDecoder
	lenDecoder     *lenDecoder
	repLenDecoder  *lenDecoder
	litProbs       []uint16
	posDecoders    []uint16

	isMatch    []uint16
	isRep      []uint16
	isRepG0    []uint16
	isRepG1    []uint16
	isRepG2    []uint16
	isRep0Long []uint16

	rep0, rep1, rep2, rep3 uint32

	state    uint32
	posState uint32
}

func newState(lc, pb, lp uint8) *state {
	s := &state{
		lc: lc,
		pb: pb,
		lp: lp,

		posMask: (1 << pb) - 1,

		lenDecoder:     newLenDecoder(),
		repLenDecoder:  newLenDecoder(),
		litProbs:       make([]uint16, uint32(0x300)<<(lc+lp)),
		posSlotDecoder: make([]*bitTreeDecoder, kNumLenToPosStates),
		posDecoders:    make([]uint16, 1+kNumFullDistances-kEndPosModelIndex),
		alignDecoder:   newBitTreeDecoder(kNumAlignBits),

		isMatch:    make([]uint16, kNumStates<<kNumPosBitsMax),
		isRep:      make([]uint16, kNumStates),
		isRepG0:    make([]uint16, kNumStates),
		isRepG1:    make([]uint16, kNumStates),
		isRepG2:    make([]uint16, kNumStates),
		isRep0Long: make([]uint16, kNumStates<<kNumPosBitsMax),
	}

	for i := 0; i < kNumLenToPosStates; i++ {
		s.posSlotDecoder[i] = newBitTreeDecoder(6)
	}

	s.Reset()

	return s
}

func (s *state) Reset() {
	s.lenDecoder.Reset()
	s.repLenDecoder.Reset()

	initProbs(s.litProbs)

	for i := 0; i < kNumLenToPosStates; i++ {
		s.posSlotDecoder[i].Reset()
	}

	initProbs(s.posDecoders)
	s.alignDecoder.Reset()

	initProbs(s.isMatch)
	initProbs(s.isRep)
	initProbs(s.isRepG0)
	initProbs(s.isRepG1)
	initProbs(s.isRepG2)
	initProbs(s.isRep0Long)

	s.rep0, s.rep1, s.rep2, s.rep3 = 0, 0, 0, 0
	s.state = 0
	s.posState = 0
}

func (s *state) SetUnpackSize(unpackSize uint64) {
	s.unpackSize = unpackSize

	s.unpackSizeDefined = isUnpackSizeDefined(unpackSize)
	s.markerIsMandatory = !s.unpackSizeDefined
}

func isUnpackSizeDefined(unpackSize uint64) bool {
	var (
		b                 byte
		unpackSizeDefined bool
	)

	for i := 0; i < 8; i++ {
		b = byte(unpackSize & 0xFF)
		if b != 0xFF {
			unpackSizeDefined = true
		}

		unpackSize >>= 8
	}

	return unpackSizeDefined
}
