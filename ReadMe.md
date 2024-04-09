# Golang LZMA reader implementation
[Format specification](https://www.7-zip.org/sdk.html)

This package based on LZMA reader from C++ code `LzmaSpec.cpp` from specification.

The reader1 and reader2 has constructor specially for [sevenzip](https://github.com/bodgit/sevenzip) package.

## Benchmark
### LZMA1 decompress
I have private 1GB tar file, compressed by lzma-utility from [xz package](https://tukaani.org/xz/).

Environment:
- os: macOS Ventura 13.6.1 (22G313)
- arch: amd64
- cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz

Decompression speed:
- 7z (21.07) - 52.37 MiB/s (+103.77%)
- xz (5.4.3) - 43.99 MiB/s (+71.17%)
- my (v0.0.1-alpha8) - 42.43 MiB/s (+65.10%)
- [ulikunitz/xz](https://github.com/ulikunitz/xz)  ([orisano](https://github.com/orisano/xz) fork at commit 4b4c597)- 25.70 MiB/s (compared with this speed)

This reader more fast than package of [ulikunitz/xz](https://github.com/ulikunitz/xz) by reducing allocations, inlining hot functions and unbranching.
