package httpapi

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	limiter := newLoginLimiter(2)
	now := time.Now()
	if !limiter.Allow("key", now) || !limiter.Allow("key", now) { t.Fatal("first requests rejected") }
	if limiter.Allow("key", now) { t.Fatal("limit not enforced") }
	if !limiter.Allow("key", now.Add(time.Minute)) { t.Fatal("window did not reset") }
}

