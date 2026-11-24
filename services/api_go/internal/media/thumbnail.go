package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
)

const thumbnailInputFormats = "image2,image2pipe,mjpeg,mjpeg_pipe,png_pipe,webp_pipe,webp_anim,gif,gif_pipe,bmp_pipe,jpeg_pipe,mov,mp4,m4a,3gp,3g2,mj2,avif,heif,heic"
const thumbnailScale = "scale=w='max(1,round(iw*min(1,480/max(iw,ih))))':h='max(1,round(ih*min(1,480/max(iw,ih))))'"

type Thumbnailer struct {
	path string
	slot chan struct{}
}

func NewThumbnailer(ffmpegPath string) *Thumbnailer {
	return &Thumbnailer{path: ffmpegPath, slot: make(chan struct{}, 1)}
}

// Ensure writes a static WebP next to the original, even when the original is an MP4.
func (t *Thumbnailer) Ensure(ctx context.Context, storage *ownedfiles.Storage, source ownedfiles.File) (ownedfiles.File, error) {
	if !ownedfiles.HasThumbnail(source.Kind) {
		return ownedfiles.File{}, os.ErrNotExist
	}
	thumbnail := source.Thumbnail()
	input, err := storage.Resolve(source.RelativePath)
	if err != nil {
		return ownedfiles.File{}, err
	}
	if info, err := os.Stat(input); err != nil || !info.Mode().IsRegular() {
		return ownedfiles.File{}, os.ErrNotExist
	}
	output, err := storage.Resolve(thumbnail.RelativePath)
	if err != nil {
		return ownedfiles.File{}, err
	}
	if hasThumbnail(output) {
		return thumbnail, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	// Serialize encoding for both the worker and legacy preview requests.
	select {
	case t.slot <- struct{}{}:
		defer func() { <-t.slot }()
	case <-ctx.Done():
		return ownedfiles.File{}, ctx.Err()
	}
	if hasThumbnail(output) {
		return thumbnail, nil
	}
	if err := t.encode(ctx, input, output); err != nil {
		return ownedfiles.File{}, err
	}
	if _, err := os.Stat(input); err != nil {
		// Expiration and deletion remove the original before their final thumbnail check.
		_ = os.Remove(output)
		return ownedfiles.File{}, err
	}
	return thumbnail, nil
}

func (t *Thumbnailer) encode(ctx context.Context, input, output string) error {
	temp, err := os.CreateTemp(filepath.Dir(output), ".thumb-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Close(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, t.path,
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-protocol_whitelist", "file", "-format_whitelist", thumbnailInputFormats,
		"-threads", "1", "-max_pixels", "67108864", "-i", input,
		"-map", "0:v:0", "-frames:v", "1", "-filter_threads", "1",
		"-vf", thumbnailScale,
		"-c:v", "libwebp", "-quality", "75", "-f", "webp", temp.Name(),
	)
	log := &limitedBuffer{limit: 4 * 1024}
	cmd.Stderr = log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v: %s", ErrFFmpeg, err, log.String())
	}
	if !hasThumbnail(temp.Name()) {
		return errors.New("ffmpeg produced an empty thumbnail")
	}
	return os.Rename(temp.Name(), output)
}

func hasThumbnail(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
