package files

import (
	"net/url"
	"testing"
	"time"
)
func TestSignerExpiresAndRejectsTampering(t *testing.T) { now := time.Now().UTC(); signer := NewSigner([]byte("01234567890123456789012345678901"), "https://capture.example.com"); signed, _ := url.Parse(signer.URL("file-1", now.Add(time.Minute))); query := signed.Query(); if !signer.Validate("file-1", query.Get("expires"), query.Get("signature"), now) { t.Fatal("valid signature rejected") }; if signer.Validate("file-2", query.Get("expires"), query.Get("signature"), now) { t.Fatal("tampered file accepted") }; if signer.Validate("file-1", query.Get("expires"), query.Get("signature"), now.Add(2*time.Minute)) { t.Fatal("expired signature accepted") } }

func TestArchiveSignerExpiresAndRejectsTampering(t *testing.T) {
	now := time.Date(2026, time.November, 22, 8, 0, 0, 0, time.UTC)
	signer := NewSigner([]byte("01234567890123456789012345678901"), "")
	signed, err := url.Parse(signer.ArchiveURL("user-1", "job-1", now.Add(5*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	query := signed.Query()
	expiry, signature := query.Get("expires"), query.Get("signature")
	if !signer.ValidateArchive("user-1", "job-1", expiry, signature, now) {
		t.Fatal("valid archive signature with an empty base URL rejected")
	}
	cases := []struct {
		name, userID, jobID, expiry, signature string
		now                                    time.Time
	}{
		{"different user", "user-2", "job-1", expiry, signature, now},
		{"different job", "user-1", "job-2", expiry, signature, now},
		{"different expiry", "user-1", "job-1", expiry + "0", signature, now},
		{"different signature", "user-1", "job-1", expiry, signature + "00", now},
		{"invalid signature", "user-1", "job-1", expiry, "not-hex", now},
		{"empty signature", "user-1", "job-1", expiry, "", now},
		{"invalid expiry", "user-1", "job-1", "invalid", signature, now},
		{"overflowed expiry", "user-1", "job-1", "99999999999999999999999999", signature, now},
		{"empty expiry", "user-1", "job-1", "", signature, now},
		{"empty user", "", "job-1", expiry, signature, now},
		{"empty job", "user-1", "", expiry, signature, now},
		{"expired", "user-1", "job-1", expiry, signature, now.Add(6 * time.Minute)},
		{"at expiry", "user-1", "job-1", expiry, signature, now.Add(5 * time.Minute)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if signer.ValidateArchive(tc.userID, tc.jobID, tc.expiry, tc.signature, tc.now) {
				t.Fatal("invalid archive signature accepted")
			}
		})
	}
	var missing *Signer
	if missing.ValidateArchive("user-1", "job-1", expiry, signature, now) {
		t.Fatal("nil signer accepted a signature")
	}
}

func TestArchiveSignerIsolatesPurposes(t *testing.T) {
	now := time.Date(2026, time.November, 22, 8, 0, 0, 0, time.UTC)
	expires := now.Add(5 * time.Minute)
	signer := NewSigner([]byte("01234567890123456789012345678901"), "https://capture.example.com")
	for name, signed := range map[string]string{
		"preview": signer.PreviewURL("job-1", expires),
		"asr":     signer.URL("job-1", expires),
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := url.Parse(signed)
			if err != nil {
				t.Fatal(err)
			}
			query := parsed.Query()
			if signer.ValidateArchive("user-1", "job-1", query.Get("expires"), query.Get("signature"), now) {
				t.Fatal("signature for another purpose accepted for archive download")
			}
		})
	}
	archive, err := url.Parse(signer.ArchiveURL("user-1", "job-1", expires))
	if err != nil {
		t.Fatal(err)
	}
	query := archive.Query()
	if signer.Validate("job-1", query.Get("expires"), query.Get("signature"), now) {
		t.Fatal("archive signature accepted for a file or preview")
	}
}

func TestArchiveSignerRelativeURLAndFieldBoundaries(t *testing.T) {
	now := time.Date(2026, time.November, 22, 8, 0, 0, 0, time.UTC)
	expires := now.Add(5 * time.Minute)
	signer := NewSigner([]byte("01234567890123456789012345678901"), "https://capture.example.com")
	userID, jobID := "user /+?&=%中文", "job /+?&=%中文"
	signed, err := url.Parse(signer.ArchiveURL(userID, jobID, expires))
	if err != nil {
		t.Fatal(err)
	}
	if signed.IsAbs() || signed.Host != "" || signed.EscapedPath() != "/api/v1/archive-download/"+url.PathEscape(jobID) {
		t.Fatalf("archive URL must use an escaped same-origin path: %q", signed.String())
	}
	query := signed.Query()
	if query.Get("user") != userID || !signer.ValidateArchive(userID, jobID, query.Get("expires"), query.Get("signature"), now) {
		t.Fatal("archive URL did not preserve its signed fields")
	}
	for _, ids := range [][4]string{
		{"a", "bc", "ab", "c"},
		{"a\x00b", "c", "a", "b\x00c"},
	} {
		original, err := url.Parse(signer.ArchiveURL(ids[0], ids[1], expires))
		if err != nil {
			t.Fatal(err)
		}
		query := original.Query()
		if signer.ValidateArchive(ids[2], ids[3], query.Get("expires"), query.Get("signature"), now) {
			t.Fatal("ambiguous signed field boundaries accepted")
		}
	}
}
