package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

var ErrFFmpeg = errors.New("ffmpeg failed")

type FFmpeg struct { Path string; Timeout time.Duration; LogLimit int }

func (f FFmpeg) ExtractMP3(ctx context.Context, workDir, input, output string) error {
	timeout := f.Timeout; if timeout <= 0 { timeout = 30*time.Minute }; limit := f.LogLimit; if limit <= 0 { limit = 16*1024 }
	runCtx, cancel := context.WithTimeout(ctx, timeout); defer cancel()
	args := []string{"-nostdin", "-hide_banner", "-y", "-i", input, "-vn", "-ar", "16000", "-ac", "1", "-b:a", "48k", output}
	cmd := exec.CommandContext(runCtx, f.Path, args...); cmd.Dir = workDir; log := &limitedBuffer{limit: limit}; cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil { return fmt.Errorf("%w: exit=%v log=%q", ErrFFmpeg, err, log.String()) }; return nil
}

type limitedBuffer struct { bytes.Buffer; limit int }
func (b *limitedBuffer) Write(value []byte) (int, error) { original := len(value); remaining := b.limit-b.Len(); if remaining > 0 { if len(value) > remaining { value = value[:remaining] }; _, _ = b.Buffer.Write(value) }; return original, nil }
