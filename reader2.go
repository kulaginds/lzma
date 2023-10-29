package lzma

import (
	"errors"
	"fmt"
	"io"
)

type Reader2 struct {
	inStream io.Reader

	dictSize uint32

	outWindow  *window
	lzmaReader *Reader1

	header                []byte
	chunkType             chunkType
	chunkUncompressedSize uint32
	chunkCompressedSize   uint16

	limitReader io.Reader
}

func NewReader2(inStream io.Reader, dictSize int) (*Reader2, error) {
	r := &Reader2{
		inStream: inStream,

		dictSize: uint32(dictSize),

		header: make([]byte, 6),
	}

	return r, r.initialize()
}

func (r *Reader2) initialize() error {
	err := r.validateDictSize()
	if err != nil {
		return err
	}

	r.outWindow = newWindow(r.dictSize)

	return r.startChunk()
}

func (r *Reader2) validateDictSize() error {
	if r.dictSize < lzmaDicMin {
		r.dictSize = 8 * 1024 * 1024
	}

	if r.dictSize > lzmaDicMax {
		return ErrDictOutOfRange
	}

	return nil
}

var chunkCounter = int64(0)

func (r *Reader2) startChunk() error {
	_, err := r.inStream.Read(r.header[0:1])
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}

		return err
	}

	r.chunkType = 0
	r.chunkUncompressedSize = 0
	r.chunkCompressedSize = 0

	r.chunkType = decodeChunkType(r.header[0])
	chunkCounter++
	opCounter = 0
	printChunk(r.chunkType)
	if r.chunkType == chunkEndOfStream {
		return nil
	}

	_, err = r.inStream.Read(r.header[1:chunkLength(r.chunkType)])
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}

		return err
	}

	r.chunkUncompressedSize = (uint32(r.header[1]) << 8) | uint32(r.header[2])
	r.chunkUncompressedSize++

	if isChunkResetDict[r.chunkType] {
		r.outWindow.Reset()
	}

	if isChunkUncompressed[r.chunkType] {
		return nil
	}

	r.chunkUncompressedSize |= uint32(r.header[0]&maskLZMAUncompressedSize) << 16
	r.chunkCompressedSize = (uint16(r.header[3]) << 8) | uint16(r.header[4])
	r.chunkCompressedSize++

	if r.lzmaReader == nil {
		r.lzmaReader, err = NewReader1WithOptions(io.LimitReader(r.inStream, int64(r.chunkCompressedSize)), r.header[5], uint64(r.chunkUncompressedSize), r.outWindow)
		if err != nil {
			return err
		}

		return nil
	}

	switch r.chunkType {
	case chunkLZMAResetState:
		r.lzmaReader.s.Reset()
	case chunkLZMAResetStateNewProp, chunkLZMAResetStateNewPropResetDict:
		lc, pb, lp := decodeProp(r.header[5])

		r.lzmaReader.s = newState(lc, pb, lp)
	}

	err = r.lzmaReader.Reopen(io.LimitReader(r.inStream, int64(r.chunkCompressedSize)), uint64(r.chunkUncompressedSize))
	if err != nil {
		return err
	}

	return nil
}

func printChunk(chunkType chunkType) {
	name := ""

	switch chunkType {
	case chunkEndOfStream:
		name = "cEOS"
	case chunkUncompressedResetDict:
		name = "cUD"
	case chunkUncompressedNoResetDict:
		name = "cU"
	case chunkLZMANoReset:
		name = "cL"
	case chunkLZMAResetState:
		name = "cLR"
	case chunkLZMAResetStateNewProp:
		name = "cLRN"
	case chunkLZMAResetStateNewPropResetDict:
		name = "cLRND"
	}

	fmt.Println(name, chunkCounter)
}

func decodeChunkType(chunkCode byte) chunkType {
	switch chunkCode {
	case endOfStreamCode:
		return chunkEndOfStream
	case uncompressedResetDict:
		return chunkUncompressedResetDict
	case uncompressedNoResetDict:
		return chunkUncompressedNoResetDict
	}

	lzmaSubCode := chunkCode >> 5

	switch lzmaSubCode {
	case maskLZMANoReset:
		return chunkLZMANoReset
	case maskLZMAResetState:
		return chunkLZMAResetState
	case maskLZMAResetStateNewProp:
		return chunkLZMAResetStateNewProp
	case maskLZMAResetStateNewPropResetDict:
		return chunkLZMAResetStateNewPropResetDict
	}

	return endOfStreamCode
}

func chunkLength(chunkType chunkType) int {
	switch chunkType {
	case chunkEndOfStream:
		return 1
	case chunkUncompressedResetDict, chunkUncompressedNoResetDict:
		return 3
	case chunkLZMANoReset, chunkLZMAResetState:
		return 5
	case chunkLZMAResetStateNewProp, chunkLZMAResetStateNewPropResetDict:
		return 6
	}

	return 1
}

func (r *Reader2) Read(p []byte) (n int, err error) {
	var k int

	//if chunkCounter == 69 {
	//	a := 4
	//	_ = a
	//}

	for n < len(p) {
		switch r.chunkType {
		case chunkEndOfStream:
			return n, io.EOF
		case chunkUncompressedResetDict, chunkUncompressedNoResetDict:
			k, err = r.uncompressedRead(p[n:])
			n += k
		case chunkLZMANoReset, chunkLZMAResetState, chunkLZMAResetStateNewProp, chunkLZMAResetStateNewPropResetDict:
			k, err = r.lzmaReader.Read(p[n:])
			n += k

		default:
			return n, fmt.Errorf("%w: %d", ErrUnexpectedLZMA2Code, r.chunkType)
		}

		if errors.Is(err, io.EOF) {
			err = r.startChunk()
			if err != nil {
				return n, err
			}

			continue
		}

		if err != nil {
			//fmt.Println(chunkCounter)
			return n, err
		}
	}

	return n, nil
}

func (r *Reader2) uncompressedRead(p []byte) (n int, err error) {
	var (
		k    int
		err2 error
	)

	if r.limitReader == nil {
		r.limitReader = io.LimitReader(r.inStream, int64(r.chunkUncompressedSize))
	}

	for {
		if r.outWindow.HasPending() {
			k, err2 = r.outWindow.ReadPending(p[n:])
			n += k
			if err2 != nil {
				return n, err2
			}
			if n >= len(p) {
				return n, nil
			}
		}

		_, err = r.outWindow.ReadFrom(r.limitReader)
		if errors.Is(err, io.EOF) {
			r.limitReader = nil

			break
		}
		if err != nil {
			return
		}
	}

	if r.outWindow.HasPending() {
		k, err2 = r.outWindow.ReadPending(p[n:])
		n += k
		if err2 != nil {
			return n, err2
		}
	}

	return
}
