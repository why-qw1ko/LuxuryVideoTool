package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	musicRefreshCooldown    = time.Minute
	musicRefreshTimeout     = 25 * time.Second
	musicRefreshConcurrency = 2
	musicRefreshCapacity    = 128
)

var (
	ErrMusicRefreshUnavailable = errors.New("music source refresh unavailable")
	ErrMusicRefreshNotReady    = errors.New("music source refresh requires a completed work")
	ErrMusicRefreshWrongWork   = errors.New("music source refresh returned a different work")
)

type MusicRefreshLimitError struct {
	RetryAfter time.Duration
}

func (e *MusicRefreshLimitError) Error() string { return "music source refresh rate limited" }

type musicRefreshState struct {
	mu      sync.Mutex
	entries map[string]*musicRefreshEntry
	active  int
}

type musicRefreshEntry struct {
	done      chan struct{}
	startedAt time.Time
	running   bool
	waiters   int
	cancel    context.CancelFunc
	music     refreshedMusic
	err       error
}

type refreshedMusic struct {
	url    string
	title  string
	artist string
}

func (s *Service) RefreshMusic(ctx context.Context, userID, jobID string) (Job, error) {
	return s.refreshMusicForJob(ctx, userID, jobID, false)
}

func (s *Service) RefreshAnyMusic(ctx context.Context, jobID string) (Job, error) {
	return s.refreshMusicForJob(ctx, "", jobID, true)
}

func (s *Service) refreshMusicForJob(ctx context.Context, userID, jobID string, admin bool) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	readJob := func() (Job, error) {
		if admin {
			return s.repo.FindAnyByID(ctx, jobID)
		}
		return s.repo.FindByID(ctx, userID, jobID)
	}
	job, err := readJob()
	if err != nil {
		return Job{}, err
	}
	if job.Status != "completed" || job.Work == nil {
		return job, ErrMusicRefreshNotReady
	}
	if s.resolver == nil {
		return job, ErrMusicRefreshUnavailable
	}
	target := strings.TrimSpace(job.Work.CanonicalURL)
	if target == "" {
		target = strings.TrimSpace(job.InputURL)
	}
	if target == "" {
		return job, ErrMusicRefreshNotReady
	}
	key := strings.TrimSpace(job.Work.DouyinWorkID)
	if key == "" {
		key = target
	}
	entry, err := s.joinMusicRefresh(ctx, key, target, job.Work.DouyinWorkID)
	if err != nil {
		return job, err
	}
	defer s.leaveMusicRefresh(entry)
	select {
	case <-ctx.Done():
		return job, ctx.Err()
	case <-entry.done:
	}
	if err := ctx.Err(); err != nil {
		return job, err
	}
	if entry.err != nil {
		return job, entry.err
	}
	fresh, err := readJob()
	if err != nil {
		return Job{}, err
	}
	if fresh.Status != "completed" || fresh.Work == nil {
		return fresh, ErrMusicRefreshNotReady
	}
	work := *fresh.Work
	work.MusicURL, work.MusicTitle, work.MusicArtist = entry.music.url, entry.music.title, entry.music.artist
	fresh.Work = &work
	return fresh, nil
}

func (s *Service) joinMusicRefresh(ctx context.Context, key, target, workID string) (*musicRefreshEntry, error) {
	state := &s.musicRefresh
	state.mu.Lock()
	defer state.mu.Unlock()
	now := s.now().UTC()
	if entry := state.entries[key]; entry != nil {
		if entry.running {
			if entry.waiters == 0 {
				return nil, &MusicRefreshLimitError{RetryAfter: max(time.Second, musicRefreshCooldown-now.Sub(entry.startedAt))}
			}
			entry.waiters++
			return entry, nil
		}
		if remaining := musicRefreshCooldown - now.Sub(entry.startedAt); remaining > 0 {
			return nil, &MusicRefreshLimitError{RetryAfter: remaining}
		}
	}
	if state.active >= musicRefreshConcurrency {
		return nil, &MusicRefreshLimitError{RetryAfter: time.Second}
	}
	for existingKey, entry := range state.entries {
		if !entry.running && !now.Before(entry.startedAt.Add(musicRefreshCooldown)) {
			delete(state.entries, existingKey)
		}
	}
	if len(state.entries) >= musicRefreshCapacity {
		retryAfter := musicRefreshCooldown
		for _, entry := range state.entries {
			remaining := entry.startedAt.Add(musicRefreshCooldown).Sub(now)
			if remaining > 0 && remaining < retryAfter {
				retryAfter = remaining
			}
		}
		return nil, &MusicRefreshLimitError{RetryAfter: retryAfter}
	}
	if state.entries == nil {
		state.entries = make(map[string]*musicRefreshEntry)
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), musicRefreshTimeout)
	entry := &musicRefreshEntry{done: make(chan struct{}), startedAt: now, running: true, waiters: 1, cancel: cancel}
	state.entries[key] = entry
	state.active++
	go func() {
		work, err := s.resolver.RefreshMusic(refreshCtx, target)
		if err == nil {
			err = refreshCtx.Err()
		}
		if err == nil && workID != "" && work.DouyinWorkID != workID {
			err = ErrMusicRefreshWrongWork
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		entry.err = err
		if err == nil {
			entry.music = refreshedMusic{url: work.MusicURL, title: work.MusicTitle, artist: work.MusicArtist}
		}
		entry.running = false
		state.active--
		cancel()
		close(entry.done)
	}()
	return entry, nil
}

func (s *Service) leaveMusicRefresh(entry *musicRefreshEntry) {
	state := &s.musicRefresh
	state.mu.Lock()
	defer state.mu.Unlock()
	entry.waiters--
	if entry.running && entry.waiters == 0 {
		entry.cancel()
	}
}
