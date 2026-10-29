package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
)

const maxRateLimitKeys = 100000

type rateBucket struct {
	count int
	reset time.Time
}

type RateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	buckets map[string]*rateBucket
	stop    chan struct{}
	once    sync.Once
}

func NewRateLimiter(limitPerMinute int) *RateLimiter {
	limiter := &RateLimiter{
		limit:   limitPerMinute,
		window:  time.Minute,
		now:     time.Now,
		buckets: make(map[string]*rateBucket),
		stop:    make(chan struct{}),
	}
	if limitPerMinute > 0 {
		go limiter.cleanupLoop()
	}
	return limiter
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	if l.limit <= 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := l.key(r)
		allowed, remaining, reset := l.allow(key)

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))

		if !allowed {
			retryAfter := int(time.Until(reset).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limit_error",
				"rate_limit_exceeded", "Too many requests. Retry after the rate limit window resets.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) Close() {
	l.once.Do(func() {
		close(l.stop)
	})
}

func (l *RateLimiter) allow(key string) (bool, int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket, exists := l.buckets[key]
	if !exists || !now.Before(bucket.reset) {
		if !exists && len(l.buckets) >= maxRateLimitKeys {
			l.cleanupLocked(now)
			if len(l.buckets) >= maxRateLimitKeys {
				return false, 0, now.Add(l.window)
			}
		}
		bucket = &rateBucket{count: 0, reset: now.Add(l.window)}
		l.buckets[key] = bucket
	}

	if bucket.count >= l.limit {
		return false, 0, bucket.reset
	}
	bucket.count++
	return true, l.limit - bucket.count, bucket.reset
}

func (l *RateLimiter) key(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		sum := sha256.Sum256([]byte(header))
		return "auth:" + hex.EncodeToString(sum[:])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}

func (l *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(2 * l.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			l.cleanupLocked(l.now())
			l.mu.Unlock()
		case <-l.stop:
			return
		}
	}
}

func (l *RateLimiter) cleanupLocked(now time.Time) {
	for key, bucket := range l.buckets {
		if !now.Before(bucket.reset) {
			delete(l.buckets, key)
		}
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			for _, candidate := range strings.Split(forwarded, ",") {
				candidate = strings.TrimSpace(candidate)
				if ip := net.ParseIP(candidate); ip != nil {
					return ip.String()
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
