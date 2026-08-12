package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SiliconFlow struct { APIKey, Endpoint, ModelName string; Client *http.Client }
func NewSiliconFlow(apiKey, endpoint, model string) *SiliconFlow { if endpoint == "" { endpoint = "https://api.siliconflow.cn/v1/audio/transcriptions" }; if model == "" { model = "FunAudioLLM/SenseVoiceSmall" }; return &SiliconFlow{APIKey: apiKey, Endpoint: endpoint, ModelName: model, Client: &http.Client{Timeout: 30*time.Minute}} }
func (s *SiliconFlow) Name() string { return "siliconflow_sensevoice" }; func (s *SiliconFlow) Model() string { return s.ModelName }
func (s *SiliconFlow) Validate(context.Context) error { if strings.TrimSpace(s.APIKey) == "" { return ErrAuth }; return nil }
func (s *SiliconFlow) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResult, error) { if err := s.Validate(ctx); err != nil { return TranscribeResult{}, err }; file, err := os.Open(request.AudioPath); if err != nil { return TranscribeResult{}, ErrInputRejected }; defer file.Close(); var body bytes.Buffer; writer := multipart.NewWriter(&body); part, err := writer.CreateFormFile("file", filepath.Base(request.AudioPath)); if err != nil { return TranscribeResult{}, err }; if _, err := io.Copy(part, file); err != nil { return TranscribeResult{}, err }; _ = writer.WriteField("model", s.ModelName); if err := writer.Close(); err != nil { return TranscribeResult{}, err }; req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, &body); req.Header.Set("Authorization", "Bearer "+s.APIKey); req.Header.Set("Content-Type", writer.FormDataContentType()); resp, err := s.Client.Do(req); if err != nil { return TranscribeResult{}, ErrFailed }; defer resp.Body.Close(); if err := statusError(resp.StatusCode, resp.Header.Get("Retry-After")); err != nil { return TranscribeResult{}, err }; var payload struct { Text string `json:"text"` }; if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload) != nil { return TranscribeResult{}, ErrFailed }; return TranscribeResult{RawText: payload.Text, RequestID: resp.Header.Get("x-siliconcloud-trace-id"), Summary: map[string]any{"textLength": len([]rune(payload.Text))}}, nil }
