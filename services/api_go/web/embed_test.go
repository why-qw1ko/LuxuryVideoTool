package web

import (
	"testing"
	"testing/fstest"
)

func TestAssetETagChangesWithContent(t *testing.T) {
	assets := fstest.MapFS{"static/app.js": &fstest.MapFile{Data: []byte("first")}}
	first, err := assetETag(assets)
	if err != nil {
		t.Fatal(err)
	}
	assets["static/app.js"].Data = []byte("other")
	second, err := assetETag(assets)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("asset content changed without invalidating the cache")
	}
}
