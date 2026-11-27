package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"sync"
	"time"
)

const (
	coverCacheTTL      = 5 * time.Minute
	coverCacheEntries  = 64
	coverCacheMaxBytes = 32 << 20
	coverFetchLimit    = 8
)

type coverCacheEntry struct {
	body    []byte
	mime    string
	expires time.Time
	used    uint64
}

type coverWaiter struct{ ctx context.Context }

type coverCall struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters map[*coverWaiter]struct{}
	body    []byte
	mime    string
	err     error
}

type coverCache struct {
	mu      sync.Mutex
	entries map[[32]byte]coverCacheEntry
	calls   map[[32]byte]*coverCall
	bytes   int
	active  int
	used    uint64
	now     func() time.Time
}

func (c *coverCache) timeNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Each caller can cancel independently. The shared fetch stops when no callers remain.
func (f *CoverFetcher) cachedFetch(ctx context.Context, candidates []string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	key := sha256.Sum256([]byte(strings.Join(candidates, "\x00")))
	c := &f.cache
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[[32]byte]coverCacheEntry)
		c.calls = make(map[[32]byte]*coverCall)
	}
	c.prune(c.timeNow())
	if entry, ok := c.entries[key]; ok {
		c.used++
		entry.used = c.used
		c.entries[key] = entry
		body := bytes.Clone(entry.body)
		c.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		return body, entry.mime, nil
	}
	waiter := &coverWaiter{ctx: ctx}
	call, shared := c.calls[key]
	if !shared {
		if c.active >= coverFetchLimit {
			c.mu.Unlock()
			return nil, "", ErrDownload
		}
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		call = &coverCall{done: make(chan struct{}), cancel: cancel, waiters: make(map[*coverWaiter]struct{})}
		c.calls[key] = call
		c.active++
		call.waiters[waiter] = struct{}{}
		go f.fetchShared(fetchCtx, key, candidates, call)
	} else {
		call.waiters[waiter] = struct{}{}
	}
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(call.waiters, waiter)
		if len(call.waiters) == 0 && c.calls[key] == call {
			delete(c.calls, key)
			call.cancel()
		}
		c.mu.Unlock()
		return nil, "", ctx.Err()
	case <-call.done:
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		return bytes.Clone(call.body), call.mime, call.err
	}
}

func (f *CoverFetcher) fetchShared(ctx context.Context, key [32]byte, candidates []string, call *coverCall) {
	defer call.cancel()
	body, mime, err := f.fetchCandidates(ctx, candidates)
	if err == nil && ctx.Err() != nil {
		body, mime, err = nil, "", ctx.Err()
	}
	c := &f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active--
	call.body, call.mime, call.err = body, mime, err
	if c.calls[key] == call {
		delete(c.calls, key)
		if err == nil {
			for waiter := range call.waiters {
				if waiter.ctx.Err() == nil {
					c.store(key, body, mime, c.timeNow())
					break
				}
			}
		}
	}
	close(call.done)
}

func (c *coverCache) prune(now time.Time) {
	for key, entry := range c.entries {
		if !entry.expires.After(now) {
			c.bytes -= len(entry.body)
			delete(c.entries, key)
		}
	}
}

func (c *coverCache) store(key [32]byte, body []byte, mime string, now time.Time) {
	if len(body) > coverCacheMaxBytes {
		return
	}
	c.prune(now)
	for len(c.entries) >= coverCacheEntries || c.bytes+len(body) > coverCacheMaxBytes {
		var oldest [32]byte
		var least uint64
		for candidate, entry := range c.entries {
			if least == 0 || entry.used < least {
				oldest, least = candidate, entry.used
			}
		}
		c.bytes -= len(c.entries[oldest].body)
		delete(c.entries, oldest)
	}
	c.used++
	c.entries[key] = coverCacheEntry{body: body, mime: mime, expires: now.Add(coverCacheTTL), used: c.used}
	c.bytes += len(body)
}
