package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Paraformer struct { APIKey, Endpoint, ModelName, VocabularyID string; Client *http.Client; PollInterval time.Duration }
func NewParaformer(apiKey, endpoint, model string) *Paraformer { if endpoint == "" { endpoint = "https://dashscope.aliyuncs.com/api/v1" }; if model == "" { model = "paraformer-v2" }; return &Paraformer{APIKey: apiKey, Endpoint: strings.TrimRight(endpoint, "/"), ModelName: model, Client: &http.Client{Timeout: 30*time.Second}, PollInterval: time.Second} }
func (p *Paraformer) Name() string { return "aliyun_paraformer" }
func (p *Paraformer) Model() string { return p.ModelName }
func (p *Paraformer) Validate(context.Context) error { if strings.TrimSpace(p.APIKey) == "" { return ErrAuth }; endpoint, err := url.Parse(p.Endpoint); if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" { return ErrAuth }; return nil }
func (p *Paraformer) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResult, error) {
	if err := p.Validate(ctx); err != nil { return TranscribeResult{}, err }; if request.AudioURL == "" { return TranscribeResult{}, ErrInputRejected }
	parameters := map[string]any{"channel_id": []int{0}, "language_hints": request.LanguageHints}
	if p.VocabularyID != "" { parameters["vocabulary_id"] = p.VocabularyID }
	body := map[string]any{"model": p.ModelName, "input": map[string]any{"file_urls": []string{request.AudioURL}}, "parameters": parameters}
	var submitted struct { Output struct { TaskID string `json:"task_id"` } `json:"output"`; RequestID string `json:"request_id"` }
	if err := p.call(ctx, http.MethodPost, p.Endpoint+"/services/audio/asr/transcription", body, true, &submitted); err != nil { return TranscribeResult{}, err }
	if submitted.Output.TaskID == "" { return TranscribeResult{}, ErrFailed }
	if request.TaskReporter != nil { if err := request.TaskReporter(submitted.Output.TaskID, submitted.RequestID); err != nil { return TranscribeResult{}, err } }
	return p.wait(ctx, request, submitted.Output.TaskID, submitted.RequestID)
}
func (p *Paraformer) Resume(ctx context.Context, request TranscribeRequest, taskID string) (TranscribeResult, error) { if taskID == "" { return TranscribeResult{}, ErrFailed }; return p.wait(ctx, request, taskID, "") }
func (p *Paraformer) wait(ctx context.Context, request TranscribeRequest, taskID, submitRequestID string) (TranscribeResult, error) {
	interval := p.PollInterval; if interval <= 0 { interval = time.Second }
	for {
		var status struct { RequestID string `json:"request_id"`; Output struct { TaskID string `json:"task_id"`; TaskStatus string `json:"task_status"`; Results []struct { TranscriptionURL string `json:"transcription_url"`; SubtaskStatus string `json:"subtask_status"`; Code string `json:"code"` } `json:"results"` } `json:"output"`; Usage struct { Duration float64 `json:"duration"` } `json:"usage"` }
		if err := p.call(ctx, http.MethodPost, p.Endpoint+"/tasks/"+url.PathEscape(taskID), nil, false, &status); err != nil { return TranscribeResult{}, err }
		if request.ProgressReporter != nil { if err := request.ProgressReporter(status.Output.TaskStatus != "PENDING" && status.Output.TaskStatus != "RUNNING"); err != nil { return TranscribeResult{}, err } }
		switch status.Output.TaskStatus {
		case "PENDING", "RUNNING": timer := time.NewTimer(interval); select { case <-ctx.Done(): timer.Stop(); return TranscribeResult{}, ctx.Err(); case <-timer.C: }
		case "SUCCEEDED":
			if len(status.Output.Results) == 0 || status.Output.Results[0].SubtaskStatus != "SUCCEEDED" { return TranscribeResult{}, ErrFailed }
			result, err := p.fetchResult(ctx, status.Output.Results[0].TranscriptionURL); if err != nil { return TranscribeResult{}, err }
			result.RequestID, result.ProviderTaskID, result.BilledSeconds = status.RequestID, taskID, status.Usage.Duration
			if result.RequestID == "" { result.RequestID = submitRequestID }
			return result, nil
		default: return TranscribeResult{}, ErrFailed
		}
	}
}
func (p *Paraformer) call(ctx context.Context, method, endpoint string, body any, async bool, target any) error { var reader io.Reader; if body != nil { encoded, err := json.Marshal(body); if err != nil { return err }; reader = bytes.NewReader(encoded) }; req, err := http.NewRequestWithContext(ctx, method, endpoint, reader); if err != nil { return err }; req.Header.Set("Authorization", "Bearer "+p.APIKey); req.Header.Set("Content-Type", "application/json"); if async { req.Header.Set("X-DashScope-Async", "enable") }; resp, err := p.Client.Do(req); if err != nil { return fmt.Errorf("%w: %v", ErrFailed, err) }; defer resp.Body.Close(); if err := statusError(resp.StatusCode, resp.Header.Get("Retry-After")); err != nil { io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)); return err }; if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil { return fmt.Errorf("%w: decode response", ErrFailed) }; return nil }
func (p *Paraformer) fetchResult(ctx context.Context, location string) (TranscribeResult, error) { parsed, err := url.Parse(location); if err != nil || parsed.Scheme != "https" || parsed.Host == "" { return TranscribeResult{}, ErrFailed }; req, _ := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil); resp, err := p.Client.Do(req); if err != nil { return TranscribeResult{}, ErrFailed }; defer resp.Body.Close(); if resp.StatusCode != http.StatusOK { return TranscribeResult{}, ErrFailed }; var payload struct { Properties struct { Duration int64 `json:"original_duration_in_milliseconds"` } `json:"properties"`; Transcripts []struct { Text string `json:"text"`; Sentences []struct { Begin int64 `json:"begin_time"`; End int64 `json:"end_time"`; Text string `json:"text"`; SpeakerID *int `json:"speaker_id"` } `json:"sentences"` } `json:"transcripts"` }; if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload) != nil { return TranscribeResult{}, ErrFailed }; result := TranscribeResult{DurationSeconds: float64(payload.Properties.Duration)/1000, Summary: map[string]any{"transcriptCount": len(payload.Transcripts)}}; for _, transcript := range payload.Transcripts { result.RawText += transcript.Text; for index, sentence := range transcript.Sentences { result.Segments = append(result.Segments, Segment{Index: index, StartMS: sentence.Begin, EndMS: sentence.End, Text: sentence.Text, SpeakerID: sentence.SpeakerID}) } }; return result, nil }
func statusError(status int, retryAfter string) error { switch { case status == http.StatusUnauthorized || status == http.StatusForbidden: return ErrAuth; case status == http.StatusPaymentRequired: return ErrBudgetExceeded; case status == http.StatusTooManyRequests: seconds, _ := strconv.Atoi(retryAfter); return rateLimitError{after: time.Duration(seconds)*time.Second}; case status >= 400 && status < 500: return ErrInputRejected; case status >= 500: return ErrFailed; default: return nil } }
