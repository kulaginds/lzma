package lzma

import (
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

			err = Decode(input, io.Discard)
			tc.checkErr(err)
		})
	}
}
