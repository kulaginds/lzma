# Golang LZMA reader implementation
[Format specification](https://www.7-zip.org/sdk.html)

This package implements LZMA reader like C++ code from `LzmaSpec.cpp` from specification.

This reader more fast then lzma package of [ulikunitz/xz](https://github.com/ulikunitz/xz) (approximately 10%) by reducing allocations and inlining hot functions.

The reader1 and reader2 has constructor specially for [sevenzip](https://github.com/bodgit/sevenzip) package.
