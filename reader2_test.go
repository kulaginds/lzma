package lzma

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestReader2WithFileVerification(t *testing.T) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma2")
	if err != nil {
		t.Fatal(err)
	}

	actualSummator := md5.New()
	r, err := NewReader2(bytes.NewReader(compressedData), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(actualSummator, r)
	actualSum := fmt.Sprintf("%x", actualSummator.Sum(nil))

	if actualSum != randomFileMD5 {
		t.Fatal("decompressed data corrupted")
	}
}

// goos: darwin
// goarch: amd64
// pkg: github.com/kulaginds/lzma
// cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
// BenchmarkReader2
// BenchmarkReader2-12    	     579	   1972529 ns/op	 531.59 MB/s	 8394749 B/op	      30 allocs/op

func BenchmarkReader2(b *testing.B) {
	compressedData, err := os.ReadFile("testassets/randomfile.dat.lzma2")
	if err != nil {
		b.Fatal(err)
	}

	var r *Reader2

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, err = NewReader2(bytes.NewReader(compressedData), 0)
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
