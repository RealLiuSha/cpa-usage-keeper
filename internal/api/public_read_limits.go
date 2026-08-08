package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Public share read limits: anti-crawl floor for multi-viewer NAT dashboards, not a hard quota.
// TRADEOFF: per-process memory map, no cross-instance coordination; maxBuckets caps XFF/peer churn.
const (
	publicReadRateLimitWindow = time.Minute
	// 1200/min ≈ 20 req/s: ~10 concurrent viewers with 10s overview refresh + tab churn.
	publicReadRateLimitMax = 1200
	// Bound map size so a rotating peer/XFF cannot grow memory without bound.
	publicReadRateLimitMaxBuckets = 4096
)

type publicReadRateBucket struct {
	count   int
	resetAt time.Time
}

// publicReadRateLimiter is constructed per NewRouter (not package-global).
type publicReadRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*publicReadRateBucket
	window  time.Duration
	max     int
	maxKeys int
	// now is overridable in tests.
	now func() time.Time
}

func newPublicReadRateLimiter() *publicReadRateLimiter {
	return &publicReadRateLimiter{
		buckets: make(map[string]*publicReadRateBucket),
		window:  publicReadRateLimitWindow,
		max:     publicReadRateLimitMax,
		maxKeys: publicReadRateLimitMaxBuckets,
		now:     time.Now,
	}
}

func publicReadNoStoreMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		setNoStoreHeaders(c)
		c.Next()
	}
}

func (l *publicReadRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if l == nil {
			c.Next()
			return
		}
		clientKey := publicReadClientKey(c)
		now := l.now()
		l.mu.Lock()
		l.evictExpiredLocked(now)
		bucket := l.buckets[clientKey]
		if bucket == nil || !now.Before(bucket.resetAt) {
			if bucket == nil && len(l.buckets) >= l.maxKeys {
				// Map at capacity after eviction: refuse this key rather than grow forever.
				l.mu.Unlock()
				c.Header("Retry-After", "60")
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
				return
			}
			bucket = &publicReadRateBucket{count: 0, resetAt: now.Add(l.window)}
			l.buckets[clientKey] = bucket
		}
		if bucket.count >= l.max {
			l.mu.Unlock()
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			return
		}
		bucket.count++
		l.mu.Unlock()
		c.Next()
	}
}

func (l *publicReadRateLimiter) evictExpiredLocked(now time.Time) {
	for key, bucket := range l.buckets {
		if bucket == nil || !now.Before(bucket.resetAt) {
			delete(l.buckets, key)
		}
	}
}

// publicReadClientKey uses the TCP peer only. Gin ClientIP may honor X-Forwarded-For
// when TrustedProxyCIDRs is set; that is client-spoofable and must not key an unbounded map.
func publicReadClientKey(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return "unknown"
	}
	if addr := c.Request.RemoteAddr; addr != "" {
		return addr
	}
	return "unknown"
}
