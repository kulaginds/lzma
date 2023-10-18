package lzma

import "errors"

var (
	ErrCorrupted           = errors.New("corrupted")
	ErrIncorrectProperties = errors.New("incorrect LZMA properties")
	ErrResultError         = errors.New("result error")
	ErrDictOutOfRange      = errors.New("dictionary capacity is out of range")
)
