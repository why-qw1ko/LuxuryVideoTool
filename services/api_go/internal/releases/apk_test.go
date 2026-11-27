package releases

import (
	"encoding/binary"
	"testing"
)

func TestStringPoolRejectsTruncatedUTF16Terminator(t *testing.T) {
	pool := make([]byte, 35)
	binary.LittleEndian.PutUint16(pool, 1)
	binary.LittleEndian.PutUint16(pool[2:], 28)
	binary.LittleEndian.PutUint32(pool[4:], uint32(len(pool)))
	binary.LittleEndian.PutUint32(pool[8:], 1)
	binary.LittleEndian.PutUint32(pool[20:], 32)
	if _, err := parseStringPool(pool); err == nil {
		t.Fatal("accepted a UTF16 string without a complete terminator")
	}
}

func TestStringPoolRejectsDecodedSizeAmplification(t *testing.T) {
	const length = 1024
	for _, utf8Encoding := range []bool{true, false} {
		name, decodedSize := "UTF16", length*3
		if utf8Encoding {
			name, decodedSize = "UTF8", length
		}
		t.Run(name, func(t *testing.T) {
			buildPool := func(count int) []byte {
				start := 28 + count*4
				encodedSize := 2 + length*2 + 2
				if utf8Encoding {
					encodedSize = 4 + length + 1
				}
				pool := make([]byte, start+encodedSize)
				binary.LittleEndian.PutUint16(pool, 1)
				binary.LittleEndian.PutUint16(pool[2:], 28)
				binary.LittleEndian.PutUint32(pool[4:], uint32(len(pool)))
				binary.LittleEndian.PutUint32(pool[8:], uint32(count))
				binary.LittleEndian.PutUint32(pool[20:], uint32(start))
				value := pool[start:]
				if utf8Encoding {
					binary.LittleEndian.PutUint32(pool[16:], 0x100)
					value[0], value[1], value[2], value[3] = 0x84, 0, 0x84, 0
					for i := range length {
						value[4+i] = 'a'
					}
				} else {
					binary.LittleEndian.PutUint16(value, length)
					for i := range length {
						binary.LittleEndian.PutUint16(value[2+i*2:], 'a')
					}
				}
				return pool
			}
			if values, err := parseStringPool(buildPool(2)); err != nil || len(values) != 2 || len(values[0]) != length {
				t.Fatalf("valid repeated string offsets: length=%d err=%v", len(values), err)
			}
			count := maxDecodedStringPoolBytes/decodedSize + 1
			if _, err := parseStringPool(buildPool(count)); err == nil {
				t.Fatal("accepted repeated offsets exceeding the decoded string budget")
			}
		})
	}
}

func FuzzParseManifest(f *testing.F) {
	f.Add([]byte{3, 0, 8, 0, 8, 0, 0, 0})
	f.Add([]byte{})
	f.Add([]byte{3, 0, 8, 0, 16, 0, 0, 0, 1, 0, 28, 0, 8, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 2<<20 {
			t.Skip()
		}
		_, _ = parseManifest(data)
	})
}

func FuzzStringPool(f *testing.F) {
	f.Add(make([]byte, 35))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 2<<20 {
			t.Skip()
		}
		_, _ = parseStringPool(data)
	})
}
