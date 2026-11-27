package media

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
)

func thumbnailRetryFixture(t *testing.T) (*ownedfiles.Storage, ownedfiles.File, string) {
	t.Helper()
	storage, err := ownedfiles.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	relative, _, path, err := storage.NewTarget("owner", "job", ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return storage, ownedfiles.File{Kind: "video", RelativePath: relative, OriginalName: "video.mp4"}, path
}

func TestThumbnailFailureBackoffSharesCorruptSourceFailures(t *testing.T) {
	storage, source, _ := thumbnailRetryFixture(t)
	thumbnailer := NewThumbnailer("unused")
	var encodes atomic.Int64
	invalid := errors.New("invalid media")
	thumbnailer.encode = func(context.Context, string, string) error {
		encodes.Add(1)
		return invalid
	}
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			if _, err := thumbnailer.Ensure(t.Context(), storage, source); !errors.Is(err, invalid) {
				t.Errorf("invalid media returned %v", err)
			}
		})
	}
	group.Wait()
	if encodes.Load() != 1 {
		t.Fatalf("corrupt source was encoded %d times", encodes.Load())
	}
}

func TestThumbnailFailureBackoffRecoversAfterExpiryOrReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "expiry", true: "replacement"}[replace], func(t *testing.T) {
			storage, source, input := thumbnailRetryFixture(t)
			thumbnailer := NewThumbnailer("unused")
			now := time.Now()
			thumbnailer.now = func() time.Time { return now }
			encodes := 0
			thumbnailer.encode = func(_ context.Context, _, output string) error {
				encodes++
				if encodes == 1 {
					return errors.New("temporary decode failure")
				}
				return os.WriteFile(output, []byte("thumbnail"), 0o600)
			}
			if _, err := thumbnailer.Ensure(t.Context(), storage, source); err == nil {
				t.Fatal("invalid source returned success")
			}
			if _, err := thumbnailer.Ensure(t.Context(), storage, source); err == nil || encodes != 1 {
				t.Fatalf("failed source was retried immediately: err=%v encodes=%d", err, encodes)
			}
			if replace {
				if err := os.WriteFile(input, []byte("replacement valid video"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				now = now.Add(thumbnailRetryDelay)
			}
			if _, err := thumbnailer.Ensure(t.Context(), storage, source); err != nil || encodes != 2 {
				t.Fatalf("thumbnail did not recover: err=%v encodes=%d", err, encodes)
			}
		})
	}
}

func TestThumbnailCancellationDoesNotDelayNextRequest(t *testing.T) {
	storage, source, _ := thumbnailRetryFixture(t)
	thumbnailer := NewThumbnailer("unused")
	started := make(chan struct{})
	var encodes atomic.Int64
	thumbnailer.encode = func(ctx context.Context, _, output string) error {
		if encodes.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		return os.WriteFile(output, []byte("thumbnail"), 0o600)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := thumbnailer.Ensure(ctx, storage, source)
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled encoding returned %v", err)
	}
	if _, err := thumbnailer.Ensure(t.Context(), storage, source); err != nil || encodes.Load() != 2 {
		t.Fatalf("canceled source did not retry immediately: err=%v encodes=%d", err, encodes.Load())
	}
}

func TestThumbnailDoesNotSurviveConcurrentSourceDeletion(t *testing.T) {
	storage, source, input := thumbnailRetryFixture(t)
	thumbnailer := NewThumbnailer("unused")
	var output string
	thumbnailer.encode = func(_ context.Context, _, target string) error {
		output = target
		if err := os.WriteFile(target, []byte("thumbnail"), 0o600); err != nil {
			return err
		}
		return os.Remove(input)
	}
	if _, err := thumbnailer.Ensure(t.Context(), storage, source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted source returned %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("thumbnail remains after source deletion: %v", err)
	}
}
