package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/media"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/results"
)

type Transcriber struct { ASR *asr.Service; FFmpeg media.FFmpeg; Probe media.Probe; Signer *ownedfiles.Signer; FileRepo *ownedfiles.Repository; Storage *ownedfiles.Storage; TempRetention time.Duration }
func (t *Transcriber) Run(ctx context.Context, repo Repository, job Job, owner string, workFile ownedfiles.File) error {
	providers := t.ASR.SelectProviders(ctx, job.ASRProvider)
	if providers.Primary == nil {
		return asr.ErrAuth
	}
	videoPath, err := t.Storage.Resolve(workFile.RelativePath)
	if err != nil {
		return err
	}
	workDir, err := os.MkdirTemp(filepath.Dir(videoPath), "asr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	if err := repo.SetStage(ctx, job.ID, owner, "extracting", 55, "正在提取音频", time.Now().UTC()); err != nil {
		return err
	}
	segments, err := t.prepareAudio(ctx, workDir, videoPath, providers.Primary.Name())
	if err != nil {
		return err
	}
	texts := make([]string, 0, len(segments))
	var final asr.TranscribeResult
	languages := job.LanguageHints
	if len(languages) == 0 {
		languages = []string{"zh", "en"}
	}
	_, primaryNeedsURL := providers.Primary.(*asr.Paraformer)
	_, fallbackNeedsURL := providers.Fallback.(*asr.Paraformer)
	for index, segment := range segments {
		current := index + 1
		progress := 70 + index*19/len(segments)
		if err := repo.SetStage(ctx, job.ID, owner, "transcribing", progress,
			fmt.Sprintf("正在识别第 %d/%d 段", current, len(segments)), time.Now().UTC()); err != nil {
			return err
		}
		var sourceFile ownedfiles.File
		var audioURL string
		if t.Signer.Enabled() && (primaryNeedsURL || fallbackNeedsURL) {
			sourceFile, err = t.registerAudio(ctx, job, segment.Path)
			if err != nil {
				return err
			}
			audioURL = t.Signer.URL(sourceFile.ID, time.Now().UTC().Add(2*time.Hour))
		}
		result, err := t.ASR.TranscribeSelectedSegment(ctx, providers, job.ID, index, segment.Path, audioURL,
			segment.End-segment.Start, languages, job.Hotwords, nil)
		if sourceFile.ID != "" {
			_ = t.Storage.Remove(sourceFile)
			_ = t.FileRepo.MarkDeleted(context.WithoutCancel(ctx), sourceFile.ID, time.Now().UTC())
			_ = repo.DeleteFilesByKind(context.WithoutCancel(ctx), job.ID, "asr_source")
		}
		if err != nil {
			return err
		}
		texts = append(texts, result.RawText)
		final = result
	}
	final.RawText = strings.Join(texts, "\n"); final.NormalizedText = asr.Merge(texts); if err := repo.SetStage(ctx, job.ID, owner, "postprocessing", 90, "正在生成结果文件", time.Now().UTC()); err != nil { return err }
	bundle, err := results.Build(*job.Work, final, final.Provider, final.Model, time.Now().UTC()); if err != nil { return err }
	files := []struct{ name string; body []byte; mime, kind string }{{"result.md", []byte(bundle.Markdown), "text/markdown; charset=utf-8", "result_markdown"}, {"result.txt", []byte(bundle.Text), "text/plain; charset=utf-8", "result_text"}, {"meta.json", bundle.Meta, "application/json", "result_meta"}}
	resultFiles := make([]JobFile, 0, len(files)); for _, content := range files { file, err := t.writeResult(ctx, job, content.name, content.kind, content.mime, content.body); if err != nil { return err }; resultFiles = append(resultFiles, JobFile{ID: file.ID, Kind: file.Kind, Name: file.OriginalName, MIMEType: file.MIMEType, SizeBytes: file.SizeBytes}) }
	result := map[string]any{
		"rawText": final.RawText, "normalizedText": final.NormalizedText, "asrProvider": final.Provider,
		"asrModel": final.Model, "providerRequestId": final.RequestID, "files": resultFiles,
	}
	return repo.CompleteTranscription(ctx, job.ID, owner, result, "转写与结果生成完成", time.Now().UTC())
}
func (t *Transcriber) prepareAudio(ctx context.Context, workDir, videoPath, provider string) ([]media.SegmentFile, error) {
	if provider == asr.DeepSeekProvider {
		paths, err := t.FFmpeg.ExtractWAVSegments(ctx, workDir, videoPath)
		if err != nil {
			return nil, err
		}
		segments := make([]media.SegmentFile, 0, len(paths))
		nextStart := time.Duration(0)
		for _, path := range paths {
			duration, err := t.Probe.Duration(ctx, path)
			if err != nil {
				return nil, err
			}
			segments = append(segments, media.SegmentFile{Path: path, Start: nextStart, End: nextStart + duration})
			nextStart += duration
		}
		return segments, nil
	}
	audio := filepath.Join(workDir, "audio.mp3")
	if err := t.FFmpeg.ExtractMP3(ctx, workDir, videoPath, audio); err != nil {
		return nil, err
	}
	duration, err := t.Probe.Duration(ctx, audio)
	if err != nil {
		return nil, err
	}
	if duration > 9*time.Minute {
		return t.FFmpeg.SplitMP3(ctx, workDir, audio, "segment-%03d.mp3", duration, 9*time.Minute, time.Second)
	}
	return []media.SegmentFile{{Path: audio, End: duration}}, nil
}
func (t *Transcriber) registerAudio(ctx context.Context, job Job, path string) (ownedfiles.File, error) { stat, err := os.Stat(path); if err != nil { return ownedfiles.File{}, err }; source, err := os.ReadFile(path); if err != nil { return ownedfiles.File{}, err }; sum := sha256.Sum256(source); sourceRelative, temporary, final, err := t.Storage.NewScopedTarget("asr", job.UserID, job.ID, ".mp3"); if err != nil { return ownedfiles.File{}, err }; if err := t.Storage.WriteAtomic(temporary, final, source); err != nil { return ownedfiles.File{}, err }; expires := time.Now().UTC().Add(2*time.Hour); file, err := ownedfiles.NewFile(time.Now().UTC(), job.UserID, job.ID, "asr_source", sourceRelative, filepath.Base(path), "audio/mpeg", hex.EncodeToString(sum[:]), stat.Size(), &expires); if err != nil { _ = os.Remove(final); return ownedfiles.File{}, err }; if err := t.FileRepo.Create(ctx, file); err != nil { _ = os.Remove(final); return ownedfiles.File{}, err }; return file, nil }
func (t *Transcriber) writeResult(ctx context.Context, job Job, name, kind, mime string, body []byte) (ownedfiles.File, error) { return writeResult(t.Storage, t.FileRepo, ctx, job, name, kind, mime, body) }
// writeResult 将结果文件落盘并登记文件索引。图文/动图（note）路径不依赖 ASR，
// 直接使用 Storage/FileRepo 即可，无需完整 Transcriber。
func writeResult(storage *ownedfiles.Storage, fileRepo *ownedfiles.Repository, ctx context.Context, job Job, name, kind, mime string, body []byte) (ownedfiles.File, error) { relative, temporary, final, err := storage.NewScopedTarget("results", job.UserID, job.ID, filepath.Ext(name)); if err != nil { return ownedfiles.File{}, err }; if err := storage.WriteAtomic(temporary, final, body); err != nil { return ownedfiles.File{}, err }; sum := sha256.Sum256(body); file, err := ownedfiles.NewFile(time.Now().UTC(), job.UserID, job.ID, kind, relative, name, mime, hex.EncodeToString(sum[:]), int64(len(body)), nil); if err != nil { _ = os.Remove(final); return ownedfiles.File{}, err }; if err := fileRepo.Create(ctx, file); err != nil { _ = os.Remove(final); return ownedfiles.File{}, err }; return file, nil }
