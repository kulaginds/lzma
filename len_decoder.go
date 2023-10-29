package lzma

type lenDecoder struct {
	choice  prob
	choice2 prob

	lowCoder  []*bitTreeDecoder
	midCoder  []*bitTreeDecoder
	highCoder *bitTreeDecoder
}

func newLenDecoder() *lenDecoder {
	d := &lenDecoder{
		choice:  probInitVal,
		choice2: probInitVal,

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
	d.choice = probInitVal
	d.choice2 = probInitVal
	d.highCoder.Reset()

	for i := 0; i < len(d.lowCoder); i++ {
		d.lowCoder[i].Reset()
		d.midCoder[i].Reset()
	}
}

func (d *lenDecoder) Decode(rc *rangeDecoder, posState uint32) uint32 {
	if rc.DecodeBit(&d.choice) == 0 {
		return d.lowCoder[posState].Decode(rc)
	}

	if rc.DecodeBit(&d.choice2) == 0 {
		return 8 + d.midCoder[posState].Decode(rc)
	}

	return 16 + d.highCoder.Decode(rc)
}
