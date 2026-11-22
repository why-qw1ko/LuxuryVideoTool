package media

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

const maxDownloadCandidates = 8

type upstreamStatusError int

func (e upstreamStatusError) Error() string {
	return fmt.Sprintf("%s: upstream status %d", ErrDownload, e)
}

func (e upstreamStatusError) Unwrap() error { return ErrDownload }

// DownloadCandidates 在同一媒体的候选地址之间依次切换；不会把动图降级为静态配图。
// 临时路径必须由本次操作独占，已有文件不会被删除或覆盖。
func DownloadCandidates(ctx context.Context, downloader Downloader, candidates []string, temporaryPath, finalPath string, maxBytes int64) (DownloadResult, error) {
	if err := ctx.Err(); err != nil {
		return DownloadResult{}, err
	}
	urls := uniqueCandidates(candidates)
	if len(urls) == 0 {
		return DownloadResult{}, fmt.Errorf("%w: no media address", ErrDownload)
	}
	if filepath.Clean(temporaryPath) == filepath.Clean(finalPath) {
		return DownloadResult{}, fmt.Errorf("%w: temporary and final paths must differ", ErrDownload)
	}
	for _, path := range []string{temporaryPath, finalPath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return DownloadResult{}, fmt.Errorf("%w: download target is unavailable", ErrDownload)
		}
	}
	// 候选链共享下载总预算；配图/音乐已有的父时限仍优先生效。
	// 不给正常传输中的大文件设置很短的单次超时，避免低带宽下载被反复截断。
	if len(urls) > 1 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, mediaDownloadTimeout(maxBytes))
		defer cancel()
	}
	lastReason := "download failed"
	for _, candidate := range urls {
		if err := ctx.Err(); err != nil {
			return DownloadResult{}, err
		}
		result, err := downloader.Download(ctx, candidate, temporaryPath, finalPath, maxBytes)
		if err == nil {
			if err := ctx.Err(); err != nil {
				// 下载器可能刚完成原子改名；取消时同时收回本次独占的成品，避免留下无索引文件。
				cleanupFailed := false
				for _, path := range []string{temporaryPath, finalPath} {
					if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						cleanupFailed = true
					}
				}
				if cleanupFailed {
					return DownloadResult{}, fmt.Errorf("%w: cannot clear cancelled download", err)
				}
				return DownloadResult{}, err
			}
			return result, nil
		}
		// 下载器通常自行清理 .part；也清理未遵守这一约定的失败实现留下的本次临时文件。
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return DownloadResult{}, fmt.Errorf("%w: cannot clear failed temporary download", ErrDownload)
		}
		if err := ctx.Err(); err != nil {
			return DownloadResult{}, err
		}
		if errors.Is(err, ErrTooLarge) {
			return DownloadResult{}, ErrTooLarge
		}
		if errors.Is(err, context.Canceled) {
			return DownloadResult{}, context.Canceled
		}
		if _, statErr := os.Lstat(finalPath); !errors.Is(statErr, os.ErrNotExist) {
			return DownloadResult{}, fmt.Errorf("%w: failed downloader left a final file", ErrDownload)
		}
		lastReason = candidateFailureReason(err)
	}
	return DownloadResult{}, fmt.Errorf("%w: all %d media addresses failed (%s)", ErrDownload, len(urls), lastReason)
}

func uniqueCandidates(candidates []string) []string {
	urls := make([]string, 0, min(len(candidates), maxDownloadCandidates))
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		urls = append(urls, candidate)
		if len(urls) == maxDownloadCandidates {
			break
		}
	}
	return urls
}

func candidateFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream timed out"
	}
	var status upstreamStatusError
	if errors.As(err, &status) {
		return fmt.Sprintf("upstream status %d", status)
	}
	if errors.Is(err, resolver.ErrURLNotAllowed) {
		return "media address was rejected"
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return "upstream timed out"
		}
		return "upstream connection failed"
	}
	return "upstream download failed"
}

func mediaDownloadTimeout(maxBytes int64) time.Duration {
	total := 2*time.Minute + time.Duration(maxBytes/(2<<20))*time.Second
	if total > 2*time.Hour {
		return 2 * time.Hour
	}
	return total
}
