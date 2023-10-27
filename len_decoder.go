package lzma

type lenDecoder struct {
	choice  uint16
	choice2 uint16

	lowCoder  []*bitTreeDecoder
	midCoder  []*bitTreeDecoder
	highCoder *bitTreeDecoder
}

func newLenDecoder() *lenDecoder {
	d := &lenDecoder{
		choice:  ProbInitVal,
		choice2: ProbInitVal,

		lowCoder:  make([]*bitTreeDecoder, 1<<kNumPosBitsMax),
		midCoder:  make([]*bitTreeDecoder, 1<<kNumPosBitsMax),
		highCoder: newBitTreeDecoder(8),
	}

	for i := 0; i < len(d.lowCoder); i++ {
		d.lowCoder[i] = newBitTreeDecoder(3)
		d.midCoder[i] = newBitTreeDecoder(3)
	}

	d.Reset()

	return d
}

func (d *lenDecoder) Reset() {
	for i := 0; i < len(d.lowCoder); i++ {
		d.lowCoder[i].Reset()
		d.midCoder[i].Reset()
	}

	d.highCoder.Reset()
}

func (d *lenDecoder) Decode(rc *rangeDecoder, posState uint32) uint32 {
	bit := rc.DecodeBit(&d.choice)
	if bit == 0 {
		return d.lowCoder[posState].Decode(rc)
	}

	bit = rc.DecodeBit(&d.choice2)
	if bit == 0 {
		bit = d.midCoder[posState].Decode(rc)

		return 8 + bit
	}

	bit = d.highCoder.Decode(rc)

	return 16 + bit
}
