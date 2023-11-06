package lzma

type state struct {
	markerIsMandatory bool
	unpackSizeDefined bool
	unpackSize        uint64
	bytesLeft         uint64

	lc, pb, lp uint8

	posMask uint32

	litProbs       []prob
	posSlotDecoder []*bitTreeDecoder
	alignDecoder   *bitTreeDecoder
	posDecoders    []prob

	isMatch    []prob
	isRep      []prob
	isRepG0    []prob
	isRepG1    []prob
	isRepG2    []prob
	isRep0Long []prob

	lenDecoder    *lenDecoder
	repLenDecoder *lenDecoder

	rep0, rep1, rep2, rep3 uint32
	state                  uint32
	posState               uint32
}

func newState(lc, pb, lp uint8) *state {
	s := &state{
		lc: lc,
		pb: pb,
		lp: lp,

		posMask: (1 << pb) - 1,

		litProbs:       make([]prob, uint32(0x300)<<(lc+lp)),
		posSlotDecoder: make([]*bitTreeDecoder, kNumLenToPosStates),

		lenDecoder:    newLenDecoder(),
		repLenDecoder: newLenDecoder(),
		alignDecoder:  newBitTreeDecoder(kNumAlignBits),

		posDecoders: make([]prob, 1+kNumFullDistances-kEndPosModelIndex),

		isMatch:    make([]prob, kNumStates<<kNumPosBitsMax),
		isRep:      make([]prob, kNumStates),
		isRepG0:    make([]prob, kNumStates),
		isRepG1:    make([]prob, kNumStates),
		isRepG2:    make([]prob, kNumStates),
		isRep0Long: make([]prob, kNumStates<<kNumPosBitsMax),
	}

	for i := 0; i < len(s.posSlotDecoder); i++ {
		s.posSlotDecoder[i] = newBitTreeDecoder(6)
	}

	s.Reset()

	return s
}

func (s *state) Renew(lc, pb, lp uint8) {
	s.lc = lc
	s.pb = pb
	s.lp = lp
	s.posMask = (1 << pb) - 1

	litProbsCount := int(0x300) << (lc + lp)
	if litProbsCount > cap(s.litProbs) {
		s.litProbs = make([]prob, litProbsCount)
	} else {
		s.litProbs = s.litProbs[:litProbsCount]
	}

	s.Reset()
}

func (s *state) Reset() {
	initProbs(s.litProbs)

	for i := 0; i < len(s.posSlotDecoder); i++ {
		s.posSlotDecoder[i].Reset()
	}

	s.alignDecoder.Reset()
	initProbs(s.posDecoders)

	initProbs(s.isMatch)
	initProbs(s.isRep)
	initProbs(s.isRepG0)
	initProbs(s.isRepG1)
	initProbs(s.isRepG2)
	initProbs(s.isRep0Long)

	s.lenDecoder.Reset()
	s.repLenDecoder.Reset()

	s.rep0, s.rep1, s.rep2, s.rep3 = 0, 0, 0, 0
	s.state = 0
	s.posState = 0
}

func (s *state) SetUnpackSize(unpackSize uint64) {
	s.bytesLeft = unpackSize
	s.unpackSize = unpackSize

	s.unpackSizeDefined = isUnpackSizeDefined(unpackSize)
	s.markerIsMandatory = !s.unpackSizeDefined
}

func (s *state) decompressed() uint64 {
	return s.unpackSize - s.bytesLeft
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

func stateUpdateLiteral(state uint32) uint32 {
	if state < 4 {
		return 0
	}

	if state < 10 {
		return state - 3
	}

	return state - 6
}

func stateUpdateMatch(state uint32) uint32 {
	if state < 7 {
		return 7
	}

	return 10
}

func stateUpdateRep(state uint32) uint32 {
	if state < 7 {
		return 8
	}

	return 11
}

func stateUpdateShortRep(state uint32) uint32 {
	if state < 7 {
		return 9
	}

	return 11
}
