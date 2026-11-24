package asr

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"time"
)

// ConnectionSampleID is reserved for the signed, synthetic probe audio.
const ConnectionSampleID = "provider-connection-sample"

// ConnectionSampleWAV builds one second of a synthetic tone as mono 16 kHz PCM.
// Providers can accept it even when the transcript is empty.
func ConnectionSampleWAV() []byte {
	const rate, size = 16000, 2 * 16000
	sample := make([]byte, 44+size)
	copy(sample, "RIFF")
	binary.LittleEndian.PutUint32(sample[4:], 36+size)
	copy(sample[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(sample[16:], 16)
	binary.LittleEndian.PutUint16(sample[20:], 1)
	binary.LittleEndian.PutUint16(sample[22:], 1)
	binary.LittleEndian.PutUint32(sample[24:], rate)
	binary.LittleEndian.PutUint32(sample[28:], 2*rate)
	binary.LittleEndian.PutUint16(sample[32:], 2)
	binary.LittleEndian.PutUint16(sample[34:], 16)
	copy(sample[36:], "data")
	binary.LittleEndian.PutUint32(sample[40:], size)
	for i := 0; i < rate; i++ {
		t := float64(i) / rate
		envelope := min(min(t/.08, 1), (1-t)/.08)
		value := int16(math.Sin(2*math.Pi*440*t) * envelope * 4000)
		binary.LittleEndian.PutUint16(sample[44+2*i:], uint16(value))
	}
	return sample
}

// ProbeProvider sends only the fixed sample and returns a classified error.
// DashScope fetches that sample through the supplied signed URL.
func ProbeProvider(ctx context.Context, provider Provider, audioURL string) error {
	if provider == nil {
		return ErrAuth
	}
	const timeout = 30 * time.Second
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request := TranscribeRequest{AudioURL: audioURL, Timeout: timeout, LanguageHints: []string{"en"}}
	if _, needsURL := provider.(*Paraformer); !needsURL {
		file, err := os.CreateTemp("", "asr-connection-*.wav")
		if err != nil {
			return ErrFailed
		}
		defer os.Remove(file.Name())
		_, writeErr := file.Write(ConnectionSampleWAV())
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return ErrFailed
		}
		request.AudioPath = file.Name()
	}
	_, err := provider.Transcribe(probeCtx, request)
	if probeCtx.Err() != nil {
		return probeCtx.Err()
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrAuth):
		return ErrAuth
	case errors.Is(err, ErrRateLimited):
		return ErrRateLimited
	case errors.Is(err, ErrBudgetExceeded):
		return ErrBudgetExceeded
	case errors.Is(err, ErrInputRejected):
		return ErrInputRejected
	default:
		return ErrFailed
	}
}
