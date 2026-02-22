package api

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type authRateLimiter struct {
	enabled bool
	limits  map[string]int

	mu       sync.Mutex
	window   int64
	counters map[string]int
}

func newAuthRateLimiter(cfg RateLimitPolicy) *authRateLimiter {
	return &authRateLimiter{
		enabled: cfg.Enabled,
		limits: map[string]int{
			"read":   cfg.ReadPerMinute,
			"export": cfg.ExportPerMinute,
			"ingest": cfg.IngestPerMinute,
		},
		window:   currentMinuteWindow(),
		counters: make(map[string]int),
	}
}

func (l *authRateLimiter) Allow(r *http.Request, action string) bool {
	if l == nil || !l.enabled {
		return true
	}
	limit := l.limits[strings.TrimSpace(action)]
	if limit <= 0 {
		return true
	}
	nowWindow := currentMinuteWindow()
	key := strings.TrimSpace(action) + "|" + requestRemoteIP(r)

	l.mu.Lock()
	defer l.mu.Unlock()
	if nowWindow != l.window {
		l.window = nowWindow
		l.counters = make(map[string]int)
	}
	l.counters[key]++
	return l.counters[key] <= limit
}

func currentMinuteWindow() int64 {
	return time.Now().UTC().Unix() / 60
}
