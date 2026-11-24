package media

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"testing"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
)

func TestThumbnailerCreatesAndRemovesVideoCover(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	relative, _, path, err := storage.NewTarget("owner", "job", ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "color=red:s=32x24", "-frames:v", "1", "-c:v", "mpeg4", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create MP4 fixture: %v: %s", err, output)
	}
	source := ownedfiles.File{Kind: "video", RelativePath: relative, OriginalName: "video.mp4"}
	thumbnail, err := NewThumbnailer(ffmpeg).Ensure(t.Context(), storage, source)
	if err != nil {
		t.Fatal(err)
	}
	thumbPath, err := storage.Resolve(thumbnail.RelativePath)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(thumbPath)
	if err != nil || http.DetectContentType(contents) != "image/webp" {
		t.Fatalf("invalid generated thumbnail: %v", err)
	}
	if err := storage.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(thumbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("thumbnail was not cleaned up: %v", err)
	}
}
