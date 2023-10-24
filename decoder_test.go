package lzma

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	r := require.New(t)

	testCases := []struct {
		name string

		inputFile string

		checkErr func(err error, msgAndArgs ...interface{})
	}{
		{
			name:      "correct_file_with_size",
			inputFile: "testassets/a.lzma",
			checkErr:  r.NoError,
		},
		{
			name:      "correct_file_with_eos",
			inputFile: "testassets/a_eos.lzma",
			checkErr:  r.NoError,
		},
		{
			name:      "correct_file_with_eos_and_size",
			inputFile: "testassets/a_eos_and_size.lzma",
			checkErr:  r.NoError,
		},
		{
			name:      "correct_file_lp1_lc2_pb1",
			inputFile: "testassets/a_lp1_lc2_pb1.lzma",
			checkErr:  r.NoError,
		},
		{
			name:      "bad_file",
			inputFile: "testassets/bad_corrupted.lzma",
			checkErr:  r.Error,
		},
		//{
		//	name:      "bad_file_with_eos_and_incorrect_size",
		//	inputFile: "testassets/bad_eos_incorrect_size.lzma",
		//	checkErr:  r.Error,
		//},
		{
			name:      "bad_file_with_incorrect_size",
			inputFile: "testassets/bad_incorrect_size.lzma",
			checkErr:  r.Error,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := os.Open(tc.inputFile)
			r.NoError(err)
			defer input.Close()

			b := bytes.NewBuffer(nil)

			err = Decode(input, b)
			tc.checkErr(err)
		})
	}
}

func TestDecodeWithFileVerification(t *testing.T) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma")
	if err != nil {
		t.Fatal(err)
	}

	decompressedData, err := os.ReadFile("testassets/randomfile.dat")
	if err != nil {
		t.Fatal(err)
	}
	decompressedDataSum := fmt.Sprintf("%x", md5.Sum(decompressedData))

	actualBuf := bytes.NewBuffer(nil)
	err = Decode(bytes.NewReader(compressedData), actualBuf)
	if err != nil {
		t.Fatal(err)
	}
	actualSum := fmt.Sprintf("%x", md5.Sum(actualBuf.Bytes()))

	if actualSum != decompressedDataSum {
		t.Fatal("decompressed data corrupted")
	}
}

func BenchmarkDecode(b *testing.B) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(compressedData)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err = Decode(bytes.NewReader(compressedData), io.Discard)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReader1(b *testing.B) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma")
	if err != nil {
		b.Fatal(err)
	}

	var r *Reader1

	b.ResetTimer()
	b.SetBytes(int64(len(compressedData)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, err = NewReader1(bytes.NewReader(compressedData))
		if err != nil {
			b.Fatal(err)
		}
		_, err = io.Copy(io.Discard, r)
		if err != nil {
			b.Fatal(err)
		}
	}
}
