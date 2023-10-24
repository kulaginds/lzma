package lzma

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
