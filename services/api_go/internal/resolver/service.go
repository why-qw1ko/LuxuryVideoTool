package resolver

import (
	"context"
	"errors"
	"time"
)

type Service struct {
	adapter Adapter
	cache   Cache
	ttl     time.Duration
	version string
	now     func() time.Time
}

func NewService(adapter Adapter, cache Cache, ttl time.Duration, version string) *Service {
	return &Service{adapter: adapter, cache: cache, ttl: ttl, version: version, now: time.Now}
}

func (s *Service) Resolve(ctx context.Context, shareText string, force bool) (Work, bool, error) {
	input, err := ExtractInput(shareText)
	if err != nil {
		return Work{}, false, err
	}
	if !force && input.WorkID != "" && s.cache != nil {
		work, cacheErr := s.cache.FindFresh(ctx, input.WorkID, s.version, s.now().Add(-s.ttl))
		if cacheErr == nil {
			return work, true, nil
		}
		if !errors.Is(cacheErr, ErrCacheMiss) {
			return Work{}, false, cacheErr
		}
	}
	work, err := s.adapter.Resolve(ctx, shareText)
	if err != nil {
		return Work{}, false, err
	}
	if s.cache != nil {
		work, err = s.cache.Save(ctx, work)
		if err != nil {
			return Work{}, false, err
		}
	}
	return work, false, nil
}

var ErrCacheMiss = errors.New("resolver cache miss")
