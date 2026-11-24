package asr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDeepSeekProbePostsOnlySyntheticWAV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tools/asr" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request method, path, or authorization")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		multipart, err := r.MultipartReader()
		if err != nil {
			t.Errorf("open multipart: %v", err)
			return
		}
		part, err := multipart.NextPart()
		if err != nil || part.FormName() != "file" || filepath.Ext(part.FileName()) != ".wav" {
			t.Errorf("expected multipart WAV file: %v", err)
			return
		}
		sample, err := io.ReadAll(part)
		if err != nil || !bytes.Equal(sample, ConnectionSampleWAV()) {
			t.Errorf("unexpected probe sample: %v", err)
		}
		if multipartPart, err := multipart.NextPart(); multipartPart != nil || err != io.EOF {
			t.Errorf("DeepSeek must only receive the file field: %v", err)
		}
		_, _ = io.WriteString(w, `{"request_id":"probe","text":"","duration_ms":1000}`)
	}))
	defer server.Close()

	sample := ConnectionSampleWAV()
	if len(sample) != 32044 || string(sample[:4]) != "RIFF" || string(sample[8:12]) != "WAVE" ||
		binary.LittleEndian.Uint32(sample[24:]) != 16000 || binary.LittleEndian.Uint16(sample[34:]) != 16 {
		t.Fatal("probe WAV must be one second of signed 16-bit, 16 kHz PCM")
	}
	if err := ProbeProvider(t.Context(), NewDeepSeek("test-key", server.URL+"/v1/tools/asr"), ""); err != nil {
		t.Fatal(err)
	}
}

func TestDeepSeekTranscribeAndNormalizeErrors(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, ConnectionSampleWAV(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		status int
		body   string
		err    error
	}{
		{http.StatusOK, `{"request_id":"asr-1","text":"你好","duration_ms":1101}`, nil},
		{http.StatusOK, `{"request_id":"asr-1"}`, ErrFailed},
		{http.StatusUnauthorized, "do not expose this body", ErrAuth},
		{http.StatusPaymentRequired, "", ErrBudgetExceeded},
		{http.StatusTooManyRequests, "", ErrRateLimited},
		{http.StatusRequestTimeout, "", ErrFailed},
		{http.StatusRequestEntityTooLarge, "", ErrInputRejected},
		{http.StatusServiceUnavailable, "", ErrFailed},
	} {
		t.Run(fmt.Sprintf("%d/%t", test.status, test.err == nil), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			result, err := NewDeepSeek("test-key", server.URL).Transcribe(t.Context(), TranscribeRequest{AudioPath: audio})
			if !errors.Is(err, test.err) {
				t.Fatalf("transcribe = %v, want %v", err, test.err)
			}
			if err == nil && (result.RequestID != "asr-1" || result.RawText != "你好" ||
				result.DurationSeconds != 1.101 || result.BilledSeconds != 2) {
				t.Fatalf("unexpected transcription: %+v", result)
			}
		})
	}
}

func TestDeepSeekRejectsOversizedFileBeforeRequest(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "*.wav")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(deepSeekFileLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("oversized audio must not be transmitted")
	}))
	defer server.Close()
	if _, err := NewDeepSeek("test-key", server.URL).Transcribe(t.Context(), TranscribeRequest{AudioPath: file.Name()}); !errors.Is(err, ErrInputRejected) {
		t.Fatalf("transcribe: %v", err)
	}
}
