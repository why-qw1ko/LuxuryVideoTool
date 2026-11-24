package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DeepSeekProvider  = "deepseek_asr"
	deepSeekEndpoint  = "https://api.deepseek.com/v1/tools/asr"
	deepSeekFileLimit = 8 << 20
)

type DeepSeek struct {
	APIKey, Endpoint string
	Client           *http.Client
}

func NewDeepSeek(apiKey, endpoint string) *DeepSeek {
	if endpoint == "" {
		endpoint = deepSeekEndpoint
	}
	return &DeepSeek{
		APIKey: apiKey, Endpoint: endpoint,
		Client: &http.Client{
			Timeout: 10 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (d *DeepSeek) Name() string  { return DeepSeekProvider }
func (d *DeepSeek) Model() string { return "tools/asr" }

func (d *DeepSeek) Validate(context.Context) error {
	if strings.TrimSpace(d.APIKey) == "" {
		return ErrAuth
	}
	return nil
}

func (d *DeepSeek) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResult, error) {
	if err := d.Validate(ctx); err != nil {
		return TranscribeResult{}, err
	}
	file, err := os.Open(request.AudioPath)
	if err != nil {
		return TranscribeResult{}, ErrInputRejected
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > deepSeekFileLimit {
		return TranscribeResult{}, ErrInputRejected
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(request.AudioPath))
	if err != nil {
		return TranscribeResult{}, ErrInputRejected
	}
	size, err := io.Copy(part, io.LimitReader(file, deepSeekFileLimit+1))
	if err != nil || size == 0 || size > deepSeekFileLimit {
		return TranscribeResult{}, ErrInputRejected
	}
	if err := writer.Close(); err != nil {
		return TranscribeResult{}, ErrFailed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.Endpoint, &body)
	if err != nil {
		return TranscribeResult{}, ErrFailed
	}
	req.Header.Set("Authorization", "Bearer "+d.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := d.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return TranscribeResult{}, ctx.Err()
		}
		return TranscribeResult{}, ErrFailed
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestTimeout {
		return TranscribeResult{}, ErrFailed
	}
	if err := statusError(response.StatusCode, response.Header.Get("Retry-After")); err != nil {
		return TranscribeResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return TranscribeResult{}, ErrFailed
	}
	var payload struct {
		RequestID  string  `json:"request_id"`
		Text       *string `json:"text"`
		DurationMS int64   `json:"duration_ms"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload) != nil ||
		payload.Text == nil || payload.DurationMS < 0 {
		return TranscribeResult{}, ErrFailed
	}
	seconds := float64(payload.DurationMS) / 1000
	return TranscribeResult{
		RawText: *payload.Text, RequestID: payload.RequestID, DurationSeconds: seconds, BilledSeconds: math.Ceil(seconds),
		Summary: map[string]any{"textLength": len([]rune(*payload.Text))},
	}, nil
}
