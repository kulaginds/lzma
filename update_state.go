package lzma

func UpdateState_Literal(state uint32) uint32 {
	if state < 4 {
		return 0
	}

	if state < 10 {
		return state - 3
	}

	return state - 6
}

func UpdateState_Match(state uint32) uint32 {
	if state < 7 {
		return 7
	}

	return 10
}

func UpdateState_Rep(state uint32) uint32 {
	if state < 7 {
		return 8
	}

	return 11
}

func UpdateState_ShortRep(state uint32) uint32 {
	if state < 7 {
		return 9
	}

	return 11
}
