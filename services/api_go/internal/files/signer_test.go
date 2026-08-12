package files

import (
	"net/url"
	"testing"
	"time"
)
func TestSignerExpiresAndRejectsTampering(t *testing.T) { now := time.Now().UTC(); signer := NewSigner([]byte("01234567890123456789012345678901"), "https://capture.example.com"); signed, _ := url.Parse(signer.URL("file-1", now.Add(time.Minute))); query := signed.Query(); if !signer.Validate("file-1", query.Get("expires"), query.Get("signature"), now) { t.Fatal("valid signature rejected") }; if signer.Validate("file-2", query.Get("expires"), query.Get("signature"), now) { t.Fatal("tampered file accepted") }; if signer.Validate("file-1", query.Get("expires"), query.Get("signature"), now.Add(2*time.Minute)) { t.Fatal("expired signature accepted") } }
