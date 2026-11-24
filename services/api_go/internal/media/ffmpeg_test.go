package media

import (
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractWAVSegmentsFitsProviderLimits(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	input := filepath.Join(t.TempDir(), "input.m4a")
	cmd := exec.CommandContext(t.Context(), ffmpeg, "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "125",
		"-c:a", "aac", input)
	if log, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make audio fixture: %v: %s", err, log)
	}
	paths, err := (FFmpeg{Path: ffmpeg}).ExtractWAVSegments(t.Context(), t.TempDir(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %d segments, want 2", len(paths))
	}
	var total time.Duration
	for _, path := range paths {
		duration, err := (Probe{Path: ffprobe}).Duration(t.Context(), path)
		if err != nil || duration <= 0 || duration > 120*time.Second {
			t.Fatalf("duration = %s, err = %v", duration, err)
		}
		total += duration
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var header [36]byte
		_, readErr := io.ReadFull(file, header[:])
		info, statErr := file.Stat()
		_ = file.Close()
		if readErr != nil || statErr != nil || info.Size() > 8<<20 ||
			string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" ||
			binary.LittleEndian.Uint16(header[20:]) != 1 ||
			binary.LittleEndian.Uint16(header[22:]) != 1 ||
			binary.LittleEndian.Uint32(header[24:]) != 16000 ||
			binary.LittleEndian.Uint16(header[34:]) != 16 {
			t.Fatalf("invalid PCM WAV segment: read=%v, stat=%v, header=%q", readErr, statErr, header)
		}
	}
	if total < 124*time.Second || total > 126*time.Second {
		t.Fatalf("segments did not preserve input duration: %s", total)
	}
}
