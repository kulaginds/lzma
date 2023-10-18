package lzma

func initProbs(probs []uint16) {
	for i := 0; i < len(probs); i++ {
		probs[i] = ProbInitVal
	}
}
