package lzma

import (
	"io"
	"unsafe"
)

func (r *Reader1) decompress(needBytesCount uint32) (err error) {
	s := r.s
	rCode := r.rangeDec.Code
	rRange := r.rangeDec.Range

	for r.outWindow.pending < needBytesCount {
		if s.unpackSizeDefined && s.bytesLeft == 0 {
			if rCode == 0 {
				err = io.EOF

				break
			}
		}

		s.posState = r.outWindow.pos & s.posMask
		state2 := (s.state << kNumPosBitsMax) + s.posState

		isMatch := false
		{ // r.rangeDec.DecodeBit(&s.isMatch[state2])
			v := (*prob)(unsafe.Pointer(uintptr(unsafe.Pointer(&s.isMatch[0])) + uintptr(state2)*unsafe.Sizeof(prob(0))))
			bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

			if rCode < bound {
				*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
				rRange = bound

				// Normalize
				if rRange < kTopValue {
					b, err := r.rangeDec.inStream.ReadByte()
					if err != nil {
						return err
					}

					rRange <<= 8
					rCode = (rCode << 8) | uint32(b)
				}
			} else {
				*v -= *v >> kNumMoveBits
				rCode -= bound
				rRange -= bound
				isMatch = true

				// Normalize
				if rRange < kTopValue {
					b, err := r.rangeDec.inStream.ReadByte()
					if err != nil {
						return err
					}

					rRange <<= 8
					rCode = (rCode << 8) | uint32(b)
				}
			}
		}

		if !isMatch { // literal
			if s.unpackSizeDefined && s.bytesLeft == 0 {
				return ErrResultError
			}

			{ // DecodeLiteral
				prevByte := uint32(0)
				if !r.outWindow.IsEmpty() {
					prevByte = uint32(r.outWindow.GetByte(1))
				}

				symbol := uint32(1)
				litState := ((r.outWindow.pos & ((1 << s.lp) - 1)) << s.lc) + (prevByte >> (8 - s.lc))
				probsPtr := uintptr(unsafe.Pointer(&s.litProbs[0])) + uintptr(0x300*litState)*unsafe.Sizeof(prob(0))

				if s.state >= 7 {
					matchByte := r.outWindow.GetByte(s.rep0 + 1)

					var matchBit uint32

					for symbol < 0x100 {
						matchBit = uint32((matchByte >> 7) & 1)
						matchByte <<= 1
						probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(((1+matchBit)<<8)+symbol)*unsafe.Sizeof(prob(0))))

						{ // rc.DecodeBit
							bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

							if rCode < bound {
								*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
								rRange = bound
								symbol <<= 1

								// Normalize
								if rRange < kTopValue {
									b, err := r.rangeDec.inStream.ReadByte()
									if err != nil {
										return err
									}

									rRange <<= 8
									rCode = (rCode << 8) | uint32(b)
								}

								if matchBit != 0 {
									break
								}
							} else {
								*probPtr -= *probPtr >> kNumMoveBits
								rCode -= bound
								rRange -= bound
								symbol = (symbol << 1) | 1

								// Normalize
								if rRange < kTopValue {
									b, err := r.rangeDec.inStream.ReadByte()
									if err != nil {
										return err
									}

									rRange <<= 8
									rCode = (rCode << 8) | uint32(b)
								}

								if matchBit != 1 {
									break
								}
							}
						}
					}
				}

				for symbol < 0x100 {
					probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(symbol)*unsafe.Sizeof(prob(0))))
					{ // rc.DecodeBit
						bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

						if rCode < bound {
							*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
							rRange = bound
							symbol <<= 1

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						} else {
							*probPtr -= *probPtr >> kNumMoveBits
							rCode -= bound
							rRange -= bound
							symbol = (symbol << 1) | 1

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						}
					}
				}

				r.outWindow.PutByte(byte(symbol - 0x100))
			}

			s.state = stateUpdateLiteral(s.state)
			s.bytesLeft--

			continue
		}

		length := uint32(0)

		isRep := false
		{ // r.rangeDec.DecodeBit(&s.isRep[s.state])
			v := (*prob)(unsafe.Pointer(uintptr(unsafe.Pointer(&s.isRep[0])) + uintptr(s.state)*unsafe.Sizeof(prob(0))))
			bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

			if rCode < bound {
				*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
				rRange = bound

				// Normalize
				if rRange < kTopValue {
					b, err := r.rangeDec.inStream.ReadByte()
					if err != nil {
						return err
					}

					rRange <<= 8
					rCode = (rCode << 8) | uint32(b)
				}
			} else {
				*v -= *v >> kNumMoveBits
				rCode -= bound
				rRange -= bound
				isRep = true

				// Normalize
				if rRange < kTopValue {
					b, err := r.rangeDec.inStream.ReadByte()
					if err != nil {
						return err
					}

					rRange <<= 8
					rCode = (rCode << 8) | uint32(b)
				}
			}
		}

		if !isRep { // simple match
			s.rep3, s.rep2, s.rep1 = s.rep2, s.rep1, s.rep0

			{ // lenDecoder.Decode
				choice := false
				{ // r.rangeDec.DecodeBit(&s.lenDecoder.choice)
					v := &s.lenDecoder.choice
					bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

					if rCode < bound {
						*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
						rRange = bound

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					} else {
						*v -= *v >> kNumMoveBits
						rCode -= bound
						rRange -= bound
						choice = true

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					}
				}

				if !choice {
					{ // s.lenDecoder.lowCoder[s.posState].Decode
						m := uint32(1)

						numBits := s.lenDecoder.lowCoder[s.posState].numBits
						probsPtr := uintptr(unsafe.Pointer(&s.lenDecoder.lowCoder[s.posState].probs[0]))

						for i := 0; i < numBits; i++ {
							probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
							{ // rc.DecodeBit
								bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

								if rCode < bound {
									*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
									rRange = bound
									m <<= 1

									// Normalize
									if rRange < kTopValue {
										b, err := r.rangeDec.inStream.ReadByte()
										if err != nil {
											return err
										}

										rRange <<= 8
										rCode = (rCode << 8) | uint32(b)
									}
								} else {
									*probPtr -= *probPtr >> kNumMoveBits
									rCode -= bound
									rRange -= bound
									m = (m << 1) | 1

									// Normalize
									if rRange < kTopValue {
										b, err := r.rangeDec.inStream.ReadByte()
										if err != nil {
											return err
										}

										rRange <<= 8
										rCode = (rCode << 8) | uint32(b)
									}
								}
							}
						}

						length = m - (uint32(1) << numBits)
					}
				} else {
					choice2 := false

					{ // r.rangeDec.DecodeBit(&s.lenDecoder.choice2)
						v := &s.lenDecoder.choice2
						bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

						if rCode < bound {
							*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
							rRange = bound

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						} else {
							*v -= *v >> kNumMoveBits
							rCode -= bound
							rRange -= bound
							choice2 = true

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						}
					}

					if !choice2 {
						{ // s.lenDecoder.midCoder[s.posState].Decode
							m := uint32(1)

							numBits := s.lenDecoder.midCoder[s.posState].numBits
							probsPtr := uintptr(unsafe.Pointer(&s.lenDecoder.midCoder[s.posState].probs[0]))

							for i := 0; i < numBits; i++ {
								probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
								{ // rc.DecodeBit
									bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

									if rCode < bound {
										*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
										rRange = bound
										m <<= 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									} else {
										*probPtr -= *probPtr >> kNumMoveBits
										rCode -= bound
										rRange -= bound
										m = (m << 1) | 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									}
								}
							}

							length = 8 + m - (uint32(1) << numBits)
						}
					} else {
						{ // s.lenDecoder.highCoder.Decode
							m := uint32(1)

							numBits := s.lenDecoder.highCoder.numBits
							probsPtr := uintptr(unsafe.Pointer(&s.lenDecoder.highCoder.probs[0]))

							for i := 0; i < numBits; i++ {
								probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
								{ // rc.DecodeBit
									bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

									if rCode < bound {
										*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
										rRange = bound
										m <<= 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									} else {
										*probPtr -= *probPtr >> kNumMoveBits
										rCode -= bound
										rRange -= bound
										m = (m << 1) | 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									}
								}
							}

							length = 16 + m - (uint32(1) << numBits)
						}
					}
				}
			}

			s.state = stateUpdateMatch(s.state)

			{ // DecodeDistance
				lenState := length
				if lenState > (kNumLenToPosStates - 1) {
					lenState = kNumLenToPosStates - 1
				}

				var posSlot uint32

				{ // s.posSlotDecoder[lenState].Decode
					m := uint32(1)

					numBits := s.posSlotDecoder[lenState].numBits
					probsPtr := uintptr(unsafe.Pointer(&s.posSlotDecoder[lenState].probs[0]))

					for i := 0; i < numBits; i++ {
						probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
						{ // rc.DecodeBit
							bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

							if rCode < bound {
								*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
								rRange = bound
								m <<= 1

								// Normalize
								if rRange < kTopValue {
									b, err := r.rangeDec.inStream.ReadByte()
									if err != nil {
										return err
									}

									rRange <<= 8
									rCode = (rCode << 8) | uint32(b)
								}
							} else {
								*probPtr -= *probPtr >> kNumMoveBits
								rCode -= bound
								rRange -= bound
								m = (m << 1) | 1

								// Normalize
								if rRange < kTopValue {
									b, err := r.rangeDec.inStream.ReadByte()
									if err != nil {
										return err
									}

									rRange <<= 8
									rCode = (rCode << 8) | uint32(b)
								}
							}
						}
					}

					posSlot = m - (uint32(1) << numBits)
				}

				if posSlot < 4 {
					s.rep0 = posSlot
				} else {
					numDirectBits := (posSlot >> 1) - 1
					dist := (2 | (posSlot & 1)) << numDirectBits

					if posSlot < kEndPosModelIndex {
						{ // BitTreeReverseDecode
							probsPtr := uintptr(unsafe.Pointer(&s.posDecoders[0])) + uintptr(dist-posSlot)*unsafe.Sizeof(prob(0))

							m := uint32(1)
							symbol := uint32(0)

							for i := uint32(0); i < numDirectBits; i++ {
								probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
								{ // rc.DecodeBit
									bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

									if rCode < bound {
										*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
										rRange = bound
										m <<= 1
										symbol |= 0 << i

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									} else {
										*probPtr -= *probPtr >> kNumMoveBits
										rCode -= bound
										rRange -= bound
										m = (m << 1) | 1
										symbol |= 1 << i

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									}
								}
							}
							dist += symbol
						}
					} else {
						var res uint32
						{ // DecodeDirectBits
							numBits := numDirectBits - kNumAlignBits

							for ; numBits > 0; numBits-- {
								rRange >>= 1
								rCode -= rRange
								t := 0 - (rCode >> 31)
								rCode += rRange & t

								if rCode == rRange {
									r.rangeDec.Corrupted = true
								}

								res <<= 1
								res += t + 1

								// Normalize
								if rRange < kTopValue {
									b, err := r.rangeDec.inStream.ReadByte()
									if err != nil {
										return err
									}

									rRange <<= 8
									rCode = (rCode << 8) | uint32(b)
								}
							}
						}

						dist += res << kNumAlignBits

						symbol := uint32(0)
						{ // BitTreeReverseDecode
							probsPtr := uintptr(unsafe.Pointer(&s.alignDecoder.probs[0]))
							numBits2 := s.alignDecoder.numBits

							m := uint32(1)

							for i := 0; i < numBits2; i++ {
								probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
								{ // rc.DecodeBit
									bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

									if rCode < bound {
										*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
										rRange = bound
										m <<= 1
										symbol |= 0 << i

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									} else {
										*probPtr -= *probPtr >> kNumMoveBits
										rCode -= bound
										rRange -= bound
										m = (m << 1) | 1
										symbol |= 1 << i

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									}
								}
							}
						}

						dist += symbol
					}

					s.rep0 = dist
				}
			}

			if s.rep0 == 0xFFFFFFFF {
				if rCode == 0 {
					if s.unpackSizeDefined && s.bytesLeft > 0 {
						return ErrResultError
					}

					err = io.EOF

					break
				} else {
					return ErrResultError
				}
			}

			if s.unpackSizeDefined && s.bytesLeft == 0 {
				return ErrResultError
			}

			if s.rep0 >= r.outWindow.size || !r.outWindow.CheckDistance(s.rep0) {
				return ErrResultError
			}
		} else { // rep match
			if s.unpackSizeDefined && s.bytesLeft == 0 {
				return ErrResultError
			}

			if r.outWindow.IsEmpty() {
				return ErrResultError
			}

			isRepG0 := false
			{ // r.rangeDec.DecodeBit(&s.isRepG0[s.state])
				v := (*prob)(unsafe.Pointer(uintptr(unsafe.Pointer(&s.isRepG0[0])) + uintptr(s.state)*unsafe.Sizeof(prob(0))))
				bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

				if rCode < bound {
					*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
					rRange = bound

					// Normalize
					if rRange < kTopValue {
						b, err := r.rangeDec.inStream.ReadByte()
						if err != nil {
							return err
						}

						rRange <<= 8
						rCode = (rCode << 8) | uint32(b)
					}
				} else {
					*v -= *v >> kNumMoveBits
					rCode -= bound
					rRange -= bound
					isRepG0 = true

					// Normalize
					if rRange < kTopValue {
						b, err := r.rangeDec.inStream.ReadByte()
						if err != nil {
							return err
						}

						rRange <<= 8
						rCode = (rCode << 8) | uint32(b)
					}
				}
			}

			if !isRepG0 { // short rep match
				isRep0Long := false
				{ // r.rangeDec.DecodeBit(&s.isRep0Long[state2])
					v := (*prob)(unsafe.Pointer(uintptr(unsafe.Pointer(&s.isRep0Long[0])) + uintptr(state2)*unsafe.Sizeof(prob(0))))
					bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

					if rCode < bound {
						*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
						rRange = bound

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					} else {
						*v -= *v >> kNumMoveBits
						rCode -= bound
						rRange -= bound
						isRep0Long = true

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					}
				}

				if !isRep0Long {
					s.state = stateUpdateShortRep(s.state)
					r.outWindow.PutByte(r.outWindow.GetByte(s.rep0 + 1))
					s.bytesLeft--

					continue
				}
			} else { // rep match
				dist := uint32(0)

				isRepG1 := false
				{ // r.rangeDec.DecodeBit(&s.isRepG1[s.state])
					v := (*prob)(unsafe.Pointer(uintptr(unsafe.Pointer(&s.isRepG1[0])) + uintptr(s.state)*unsafe.Sizeof(prob(0))))
					bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

					if rCode < bound {
						*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
						rRange = bound

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					} else {
						*v -= *v >> kNumMoveBits
						rCode -= bound
						rRange -= bound
						isRepG1 = true

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					}
				}

				if !isRepG1 {
					dist = s.rep1
				} else {
					isRepG2 := false
					{ // r.rangeDec.DecodeBit(&s.isRepG2[s.state])
						v := (*prob)(unsafe.Pointer(uintptr(unsafe.Pointer(&s.isRepG2[0])) + uintptr(s.state)*unsafe.Sizeof(prob(0))))
						bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

						if rCode < bound {
							*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
							rRange = bound

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						} else {
							*v -= *v >> kNumMoveBits
							rCode -= bound
							rRange -= bound
							isRepG2 = true

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						}
					}

					if !isRepG2 {
						dist = s.rep2
					} else {
						dist = s.rep3
						s.rep3 = s.rep2
					}

					s.rep2 = s.rep1
				}

				s.rep1 = s.rep0
				s.rep0 = dist
			}

			{ // r.s.repLenDecoder.Decode
				choice := false
				{ // r.rangeDec.DecodeBit(&r.s.repLenDecoder.choice)
					v := &s.repLenDecoder.choice
					bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

					if rCode < bound {
						*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
						rRange = bound

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					} else {
						*v -= *v >> kNumMoveBits
						rCode -= bound
						rRange -= bound
						choice = true

						// Normalize
						if rRange < kTopValue {
							b, err := r.rangeDec.inStream.ReadByte()
							if err != nil {
								return err
							}

							rRange <<= 8
							rCode = (rCode << 8) | uint32(b)
						}
					}
				}

				if !choice {
					{ // r.s.repLenDecoder.lowCoder[s.posState].Decode
						m := uint32(1)

						numBits := s.repLenDecoder.lowCoder[s.posState].numBits
						probsPtr := uintptr(unsafe.Pointer(&s.repLenDecoder.lowCoder[s.posState].probs[0]))

						for i := 0; i < numBits; i++ {
							probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
							{ // rc.DecodeBit
								bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

								if rCode < bound {
									*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
									rRange = bound
									m <<= 1

									// Normalize
									if rRange < kTopValue {
										b, err := r.rangeDec.inStream.ReadByte()
										if err != nil {
											return err
										}

										rRange <<= 8
										rCode = (rCode << 8) | uint32(b)
									}
								} else {
									*probPtr -= *probPtr >> kNumMoveBits
									rCode -= bound
									rRange -= bound
									m = (m << 1) | 1

									// Normalize
									if rRange < kTopValue {
										b, err := r.rangeDec.inStream.ReadByte()
										if err != nil {
											return err
										}

										rRange <<= 8
										rCode = (rCode << 8) | uint32(b)
									}
								}
							}
						}

						length = m - (uint32(1) << numBits)
					}
				} else {
					choice2 := false
					{ // r.rangeDec.DecodeBit(&r.s.repLenDecoder.choice2)
						v := &s.repLenDecoder.choice2
						bound := (rRange >> kNumBitModelTotalBits) * uint32(*v)

						if rCode < bound {
							*v += ((1 << kNumBitModelTotalBits) - *v) >> kNumMoveBits
							rRange = bound

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						} else {
							*v -= *v >> kNumMoveBits
							rCode -= bound
							rRange -= bound
							choice2 = true

							// Normalize
							if rRange < kTopValue {
								b, err := r.rangeDec.inStream.ReadByte()
								if err != nil {
									return err
								}

								rRange <<= 8
								rCode = (rCode << 8) | uint32(b)
							}
						}
					}

					if !choice2 {
						{ // r.s.repLenDecoder.midCoder[s.posState].Decode
							m := uint32(1)

							numBits := s.repLenDecoder.midCoder[s.posState].numBits
							probsPtr := uintptr(unsafe.Pointer(&s.repLenDecoder.midCoder[s.posState].probs[0]))

							for i := 0; i < numBits; i++ {
								probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
								{ // rc.DecodeBit
									bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

									if rCode < bound {
										*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
										rRange = bound
										m <<= 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									} else {
										*probPtr -= *probPtr >> kNumMoveBits
										rCode -= bound
										rRange -= bound
										m = (m << 1) | 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									}
								}
							}

							length = 8 + m - (uint32(1) << numBits)
						}
					} else {
						{ // r.s.repLenDecoder.highCoder.Decode
							m := uint32(1)

							numBits := s.repLenDecoder.highCoder.numBits
							probsPtr := uintptr(unsafe.Pointer(&s.repLenDecoder.highCoder.probs[0]))

							for i := 0; i < numBits; i++ {
								probPtr := (*prob)(unsafe.Pointer(probsPtr + uintptr(m)*unsafe.Sizeof(prob(0))))
								{ // rc.DecodeBit
									bound := (rRange >> kNumBitModelTotalBits) * uint32(*probPtr)

									if rCode < bound {
										*probPtr += ((1 << kNumBitModelTotalBits) - *probPtr) >> kNumMoveBits
										rRange = bound
										m <<= 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									} else {
										*probPtr -= *probPtr >> kNumMoveBits
										rCode -= bound
										rRange -= bound
										m = (m << 1) | 1

										// Normalize
										if rRange < kTopValue {
											b, err := r.rangeDec.inStream.ReadByte()
											if err != nil {
												return err
											}

											rRange <<= 8
											rCode = (rCode << 8) | uint32(b)
										}
									}
								}
							}

							length = 16 + m - (uint32(1) << numBits)
						}
					}
				}
			}

			s.state = stateUpdateRep(s.state)
		}

		length += kMatchMinLen
		isError := false
		if s.unpackSizeDefined && uint32(s.bytesLeft) < length {
			length = uint32(s.bytesLeft)
			isError = true
		}

		r.outWindow.CopyMatch(s.rep0+1, length)
		s.bytesLeft -= uint64(length)

		if isError {
			return ErrResultError
		}
	}

	r.rangeDec.Code = rCode
	r.rangeDec.Range = rRange

	return
}
