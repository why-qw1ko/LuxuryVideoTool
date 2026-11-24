package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sync"
)

// Files contains the dependency-free browser client served by the API process.
//
//go:embed static/*
var Files embed.FS

// StaticETag identifies the embedded files, including local builds with no commit metadata.
var StaticETag = sync.OnceValue(func() string {
	tag, err := assetETag(Files)
	if err != nil {
		panic("read embedded web assets: " + err.Error())
	}
	return tag
})

func assetETag(assets fs.FS) (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(assets, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		hash.Write(sum[:])
		return nil
	})
	if err != nil {
		return "", err
	}
	return `W/"` + hex.EncodeToString(hash.Sum(nil)) + `"`, nil
}
