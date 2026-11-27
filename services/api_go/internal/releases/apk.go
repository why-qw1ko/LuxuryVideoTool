package releases

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

const PackageName = "com.buleng.luxuryvideo"

const maxDecodedStringPoolBytes = 8 << 20

type apkIdentity struct {
	PackageName string
	VersionCode int
	VersionName string
}

func inspectAPK(path string) (apkIdentity, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return apkIdentity{}, ErrInvalidAPK
	}
	defer archive.Close()
	if len(archive.File) > 65536 {
		return apkIdentity{}, ErrInvalidAPK
	}
	var manifest *zip.File
	dex := false
	for _, file := range archive.File {
		if file.Name == "AndroidManifest.xml" {
			if manifest != nil || file.UncompressedSize64 > 2<<20 {
				return apkIdentity{}, ErrInvalidAPK
			}
			manifest = file
		}
		if file.Name == "classes.dex" && file.UncompressedSize64 > 0 {
			dex = true
		}
	}
	if manifest == nil || !dex {
		return apkIdentity{}, ErrInvalidAPK
	}
	reader, err := manifest.Open()
	if err != nil {
		return apkIdentity{}, ErrInvalidAPK
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return apkIdentity{}, ErrInvalidAPK
	}
	identity, err := parseManifest(data)
	if err != nil || identity.PackageName != PackageName || identity.VersionCode < 1 || identity.VersionCode > 2100000000 || identity.VersionName == "" || len(identity.VersionName) > 128 {
		return apkIdentity{}, ErrInvalidAPK
	}
	return identity, nil
}

// parseManifest reads Android's compiled resource XML; resource references for
// versionName are rejected because a release must expose its actual version.
func parseManifest(data []byte) (apkIdentity, error) {
	invalid := errors.New("invalid compiled Android manifest")
	if len(data) < 8 || binary.LittleEndian.Uint16(data) != 3 || binary.LittleEndian.Uint16(data[2:]) != 8 || int(binary.LittleEndian.Uint32(data[4:])) != len(data) {
		return apkIdentity{}, invalid
	}
	var pool []string
	for offset := 8; offset < len(data); {
		if len(data)-offset < 8 {
			return apkIdentity{}, invalid
		}
		chunk := data[offset:]
		header, size := int(binary.LittleEndian.Uint16(chunk[2:])), int(binary.LittleEndian.Uint32(chunk[4:]))
		if header < 8 || size < header || size > len(chunk) {
			return apkIdentity{}, invalid
		}
		chunk = chunk[:size]
		switch binary.LittleEndian.Uint16(chunk) {
		case 1:
			var err error
			pool, err = parseStringPool(chunk)
			if err != nil {
				return apkIdentity{}, invalid
			}
		case 0x0102:
			if header < 16 || size-header < 20 {
				return apkIdentity{}, invalid
			}
			ext := chunk[header:]
			lookup := func(index uint32) (string, bool) {
				if uint64(index) >= uint64(len(pool)) {
					return "", false
				}
				return pool[index], true
			}
			name, ok := lookup(binary.LittleEndian.Uint32(ext[4:]))
			if !ok || name != "manifest" {
				return apkIdentity{}, invalid
			}
			start, stride, count := int(binary.LittleEndian.Uint16(ext[8:])), int(binary.LittleEndian.Uint16(ext[10:])), int(binary.LittleEndian.Uint16(ext[12:]))
			if start < 20 || stride < 20 || count > 1024 || start > len(ext) || count*stride > len(ext)-start {
				return apkIdentity{}, invalid
			}
			values := map[string]string{}
			for i := 0; i < count; i++ {
				attribute := ext[start+i*stride : start+(i+1)*stride]
				key, ok := lookup(binary.LittleEndian.Uint32(attribute[4:]))
				if !ok {
					return apkIdentity{}, invalid
				}
				if key != "package" && key != "versionCode" && key != "versionName" && key != "versionCodeMajor" && key != "split" {
					continue
				}
				namespace := binary.LittleEndian.Uint32(attribute)
				if key == "package" || key == "split" {
					if namespace != 0xffffffff {
						return apkIdentity{}, invalid
					}
				} else if value, ok := lookup(namespace); !ok || value != "http://schemas.android.com/apk/res/android" {
					return apkIdentity{}, invalid
				}
				if _, duplicate := values[key]; duplicate || binary.LittleEndian.Uint16(attribute[12:]) != 8 {
					return apkIdentity{}, invalid
				}
				kind, value := attribute[15], binary.LittleEndian.Uint32(attribute[16:])
				switch kind {
				case 3:
					text, ok := lookup(value)
					if !ok {
						return apkIdentity{}, invalid
					}
					values[key] = text
				case 0x10, 0x11:
					values[key] = strconv.FormatUint(uint64(value), 10)
				default:
					return apkIdentity{}, invalid
				}
			}
			code, err := strconv.Atoi(values["versionCode"])
			if err != nil || (values["versionCodeMajor"] != "" && values["versionCodeMajor"] != "0") || values["split"] != "" {
				return apkIdentity{}, invalid
			}
			return apkIdentity{PackageName: values["package"], VersionCode: code, VersionName: values["versionName"]}, nil
		}
		offset += size
	}
	return apkIdentity{}, invalid
}

func parseStringPool(chunk []byte) ([]string, error) {
	invalid := errors.New("invalid Android string pool")
	if len(chunk) < 28 {
		return nil, invalid
	}
	header := int(binary.LittleEndian.Uint16(chunk[2:]))
	count := int(binary.LittleEndian.Uint32(chunk[8:]))
	flags := binary.LittleEndian.Uint32(chunk[16:])
	start := int(binary.LittleEndian.Uint32(chunk[20:]))
	if header < 28 || header > len(chunk) || count > 65536 || count*4 > len(chunk)-header || start < header+count*4 || start > len(chunk) {
		return nil, invalid
	}
	pool := make([]string, count)
	decodedBytes := 0
	for i := range pool {
		offset := int(binary.LittleEndian.Uint32(chunk[header+i*4:]))
		if offset < 0 || offset > len(chunk)-start {
			return nil, invalid
		}
		value := chunk[start+offset:]
		if flags&0x100 != 0 {
			_, used, ok := stringLength(value, true)
			if !ok {
				return nil, invalid
			}
			length, used2, ok := stringLength(value[used:], true)
			used += used2
			if !ok || length >= len(value)-used || value[used+length] != 0 {
				return nil, invalid
			}
			if length > maxDecodedStringPoolBytes-decodedBytes {
				return nil, invalid
			}
			decodedBytes += length
			pool[i] = string(value[used : used+length])
			if !utf8.ValidString(pool[i]) {
				return nil, invalid
			}
		} else {
			length, used, ok := stringLength(value, false)
			if !ok || len(value)-used < 2 || length > (len(value)-used-2)/2 {
				return nil, invalid
			}
			// One UTF-16 unit produces at most three UTF-8 bytes.
			if length > (maxDecodedStringPoolBytes-decodedBytes)/3 {
				return nil, invalid
			}
			decodedBytes += length * 3
			units := make([]uint16, length)
			for j := range units {
				units[j] = binary.LittleEndian.Uint16(value[used+j*2:])
			}
			if binary.LittleEndian.Uint16(value[used+length*2:]) != 0 {
				return nil, invalid
			}
			pool[i] = string(utf16.Decode(units))
		}
	}
	return pool, nil
}

func stringLength(value []byte, utf8 bool) (int, int, bool) {
	if utf8 {
		if len(value) < 1 {
			return 0, 0, false
		}
		if value[0]&0x80 == 0 {
			return int(value[0]), 1, true
		}
		if len(value) < 2 {
			return 0, 0, false
		}
		return int(value[0]&0x7f)<<8 | int(value[1]), 2, true
	}
	if len(value) < 2 {
		return 0, 0, false
	}
	first := binary.LittleEndian.Uint16(value)
	if first&0x8000 == 0 {
		return int(first), 2, true
	}
	if len(value) < 4 {
		return 0, 0, false
	}
	return int(first&0x7fff)<<16 | int(binary.LittleEndian.Uint16(value[2:])), 4, true
}
