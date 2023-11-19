package lzma

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReader1(t *testing.T) {
	r := require.New(t)

	testCases := []struct {
		name string

		inputFile string

		checkErr1 func(err error, msgAndArgs ...interface{})
		checkErr2 func(err error, msgAndArgs ...interface{})
	}{
		{
			name:      "correct_file_with_size",
			inputFile: "testassets/a.lzma",
			checkErr1: r.NoError,
			checkErr2: r.NoError,
		},
		{
			name:      "correct_file_with_eos",
			inputFile: "testassets/a_eos.lzma",
			checkErr1: r.NoError,
			checkErr2: r.NoError,
		},
		{
			name:      "correct_file_with_eos_and_size",
			inputFile: "testassets/a_eos_and_size.lzma",
			checkErr1: r.NoError,
			checkErr2: r.NoError,
		},
		{
			name:      "correct_file_lp1_lc2_pb1",
			inputFile: "testassets/a_lp1_lc2_pb1.lzma",
			checkErr1: r.NoError,
			checkErr2: r.NoError,
		},
		{
			name:      "bad_file",
			inputFile: "testassets/bad_corrupted.lzma",
			checkErr1: r.NoError,
			checkErr2: r.Error,
		},
		{
			name:      "bad_file_with_eos_and_incorrect_size",
			inputFile: "testassets/bad_eos_incorrect_size.lzma",
			checkErr1: r.NoError,
			checkErr2: r.Error,
		},
		{
			name:      "bad_file_with_incorrect_size",
			inputFile: "testassets/bad_incorrect_size.lzma",
			checkErr1: r.NoError,
			checkErr2: r.Error,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := os.Open(tc.inputFile)
			r.NoError(err)
			defer input.Close()

			reader, err := NewReader1(bufio.NewReader(input))
			tc.checkErr1(err)

			_, err = io.Copy(io.Discard, reader)
			tc.checkErr2(err)
		})
	}
}

func TestReader1WithFileVerification(t *testing.T) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma")
	if err != nil {
		t.Fatal(err)
	}

	actualSummator := md5.New()
	r, err := NewReader1(bytes.NewReader(compressedData))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(actualSummator, r)
	if err != nil {
		t.Fatal(err)
	}
	actualSum := fmt.Sprintf("%x", actualSummator.Sum(nil))

	if actualSum != randomFileMD5 {
		t.Fatal("decompressed data corrupted")
	}
}

const randomFileMD5 = "b2d18c4275c394a729607ff9fe0caae7"

// goos: darwin
// goarch: amd64
// pkg: github.com/kulaginds/lzma
// cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
// BenchmarkReader1
// BenchmarkReader1-12    	      18	  65852121 ns/op	  15.92 MB/s	 8411202 B/op	     164 allocs/op

func BenchmarkReader1(b *testing.B) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma")
	if err != nil {
		b.Fatal(err)
	}

	var r *Reader1

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, err = NewReader1(bytes.NewReader(compressedData))
		if err != nil {
			b.Fatal(err)
		}

		var n int64
		n, err = io.Copy(io.Discard, r)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(n)
	}
}
