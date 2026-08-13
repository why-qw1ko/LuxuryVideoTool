package resolver

// SM3 国密哈希（GB/T 32905-2016）纯 Go 实现。
// 抖音 a_bogus 签名用 SM3(params+"cus") 生成参数指纹，需与 JS 端 sm-crypto 输出一致。

import "encoding/binary"

var sm3IV = [8]uint32{
	0x7380166F, 0x4914B2B9, 0x172442D7, 0xDA8A0600,
	0xA96F30BC, 0x163138AA, 0xE38DEE4D, 0xB0FB0E4E,
}

func rotl(x uint32, n uint) uint32 { return x<<n | x>>(32-n) }

func sm3FF(j int, x, y, z uint32) uint32 {
	if j < 16 {
		return x ^ y ^ z
	}
	return (x & y) | (x & z) | (y & z)
}

func sm3GG(j int, x, y, z uint32) uint32 {
	if j < 16 {
		return x ^ y ^ z
	}
	return (x & y) | (^x & z)
}

func sm3P0(x uint32) uint32 { return x ^ rotl(x, 9) ^ rotl(x, 17) }
func sm3P1(x uint32) uint32 { return x ^ rotl(x, 15) ^ rotl(x, 23) }

// sm3Sum 计算 data 的 SM3 摘要，返回 32 字节。
func sm3Sum(data []byte) [32]byte {
	// 填充：0x80 + 0x00... + 64 位大端比特长度
	bitLen := uint64(len(data)) * 8
	padded := make([]byte, 0, len(data)+64)
	padded = append(padded, data...)
	padded = append(padded, 0x80)
	for (len(padded) % 64) != 56 {
		padded = append(padded, 0x00)
	}
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], bitLen)
	padded = append(padded, lenBuf[:]...)

	var v [8]uint32
	copy(v[:], sm3IV[:])

	block := func(blockBytes []byte) {
		var w [68]uint32
		for i := 0; i < 16; i++ {
			w[i] = binary.BigEndian.Uint32(blockBytes[i*4:])
		}
		for j := 16; j < 68; j++ {
			w[j] = sm3P1(w[j-16]^w[j-9]^rotl(w[j-3], 15)) ^ rotl(w[j-13], 7) ^ w[j-6]
		}
		var w1 [64]uint32
		for j := 0; j < 64; j++ {
			w1[j] = w[j] ^ w[j+4]
		}
		a, b, c, d := v[0], v[1], v[2], v[3]
		e, f, g, h := v[4], v[5], v[6], v[7]
		for j := 0; j < 64; j++ {
			var t uint32
			if j < 16 {
				t = 0x79CC4519
			} else {
				t = 0x7A879D8A
			}
			ss1 := rotl(rotl(a, 12)+e+rotl(t, uint(j%32)), 7)
			ss2 := ss1 ^ rotl(a, 12)
			tt1 := sm3FF(j, a, b, c) + d + ss2 + w1[j]
			tt2 := sm3GG(j, e, f, g) + h + ss1 + w[j]
			d = c
			c = rotl(b, 9)
			b = a
			a = tt1
			h = g
			g = rotl(f, 19)
			f = e
			e = sm3P0(tt2)
		}
		v[0] ^= a
		v[1] ^= b
		v[2] ^= c
		v[3] ^= d
		v[4] ^= e
		v[5] ^= f
		v[6] ^= g
		v[7] ^= h
	}

	for i := 0; i < len(padded); i += 64 {
		block(padded[i : i+64])
	}
	var out [32]byte
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:], v[i])
	}
	return out
}

// sm3Hex 返回 data 的 SM3 摘要十六进制字符串。
func sm3Hex(data []byte) string {
	const hexDigits = "0123456789abcdef"
	sum := sm3Sum(data)
	out := make([]byte, 64)
	for i, b := range sum {
		out[i*2] = hexDigits[b>>4]
		out[i*2+1] = hexDigits[b&0x0F]
	}
	return string(out)
}
