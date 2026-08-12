package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ErrFFmpeg = errors.New("ffmpeg failed")

type FFmpeg struct { Path string; Timeout time.Duration; LogLimit int }
type Probe struct { Path string; Timeout time.Duration }

type SegmentFile struct { Path string; Start time.Duration; End time.Duration }

func (f FFmpeg) ExtractMP3(ctx context.Context, workDir, input, output string) error {
	timeout := f.Timeout; if timeout <= 0 { timeout = 30*time.Minute }; limit := f.LogLimit; if limit <= 0 { limit = 16*1024 }
	runCtx, cancel := context.WithTimeout(ctx, timeout); defer cancel()
	args := []string{"-nostdin", "-hide_banner", "-y", "-i", input, "-vn", "-ar", "16000", "-ac", "1", "-b:a", "48k", output}
	cmd := exec.CommandContext(runCtx, f.Path, args...); cmd.Dir = workDir; log := &limitedBuffer{limit: limit}; cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil { return fmt.Errorf("%w: exit=%v log=%q", ErrFFmpeg, err, log.String()) }; return nil
}

func (f FFmpeg) SplitMP3(ctx context.Context, workDir, input, pattern string, total, segment, overlap time.Duration) ([]SegmentFile, error) {
	if segment <= 0 { segment = 9*time.Minute }; if overlap < 0 || overlap >= segment { overlap = time.Second }
	step := segment-overlap; var result []SegmentFile
	timeout := f.Timeout; if timeout <= 0 { timeout = 30*time.Minute }
	for index, start := 0, time.Duration(0); start < total; index, start = index+1, start+step { length := min(segment, total-start); name := fmt.Sprintf(pattern, index); args := []string{"-nostdin", "-hide_banner", "-y", "-ss", fmt.Sprintf("%.3f", start.Seconds()), "-t", fmt.Sprintf("%.3f", length.Seconds()), "-i", input, "-ar", "16000", "-ac", "1", "-b:a", "48k", name}; runCtx, cancel := context.WithTimeout(ctx, timeout); cmd := exec.CommandContext(runCtx, f.Path, args...); cmd.Dir = workDir; log := &limitedBuffer{limit: f.LogLimit}; if log.limit <= 0 { log.limit = 16*1024 }; cmd.Stdout, cmd.Stderr = log, log; err := cmd.Run(); cancel(); if err != nil { return nil, fmt.Errorf("%w: exit=%v log=%q", ErrFFmpeg, err, log.String()) }; result = append(result, SegmentFile{Path: filepath.Join(workDir, name), Start: start, End: start+length}) }
	return result, nil
}
func (p Probe) Duration(ctx context.Context, input string) (time.Duration, error) { timeout := p.Timeout; if timeout <= 0 { timeout = time.Minute }; runCtx, cancel := context.WithTimeout(ctx, timeout); defer cancel(); output, err := exec.CommandContext(runCtx, p.Path, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", input).Output(); if err != nil { return 0, fmt.Errorf("probe duration: %w", err) }; seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64); if err != nil || seconds <= 0 { return 0, ErrFFmpeg }; return time.Duration(seconds*float64(time.Second)), nil }

type limitedBuffer struct { bytes.Buffer; limit int }
func (b *limitedBuffer) Write(value []byte) (int, error) { original := len(value); remaining := b.limit-b.Len(); if remaining > 0 { if len(value) > remaining { value = value[:remaining] }; _, _ = b.Buffer.Write(value) }; return original, nil }
