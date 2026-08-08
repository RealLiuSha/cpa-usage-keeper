package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPublicReadRateLimiterIsPerInstanceAndEvictsExpired(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	limiter := newPublicReadRateLimiter()
	limiter.max = 2
	limiter.maxKeys = 2
	limiter.now = func() time.Time { return now }

	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/public/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	hit := func(remote string) int {
		req := httptest.NewRequest(http.MethodGet, "/public/ping", nil)
		req.RemoteAddr = remote
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp.Code
	}

	if code := hit("10.0.0.1:1111"); code != http.StatusOK {
		t.Fatalf("first hit: %d", code)
	}
	if code := hit("10.0.0.1:1111"); code != http.StatusOK {
		t.Fatalf("second hit: %d", code)
	}
	if code := hit("10.0.0.1:1111"); code != http.StatusTooManyRequests {
		t.Fatalf("third hit should 429, got %d", code)
	}

	// Other peer has its own budget.
	if code := hit("10.0.0.2:2222"); code != http.StatusOK {
		t.Fatalf("other peer first hit: %d", code)
	}

	// Window expires: buckets evicted, same peer allowed again.
	now = now.Add(2 * time.Minute)
	if code := hit("10.0.0.1:1111"); code != http.StatusOK {
		t.Fatalf("after window: %d", code)
	}

	// Map capacity: two keys fill the map; third new peer is rejected rather than growing forever.
	limiter2 := newPublicReadRateLimiter()
	limiter2.max = 100
	limiter2.maxKeys = 2
	limiter2.now = func() time.Time { return now }
	router2 := gin.New()
	router2.Use(limiter2.Middleware())
	router2.GET("/public/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	hit2 := func(remote string) int {
		req := httptest.NewRequest(http.MethodGet, "/public/ping", nil)
		req.RemoteAddr = remote
		resp := httptest.NewRecorder()
		router2.ServeHTTP(resp, req)
		return resp.Code
	}
	if hit2("1.1.1.1:1") != http.StatusOK || hit2("2.2.2.2:2") != http.StatusOK {
		t.Fatal("expected first two peers to enter map")
	}
	if code := hit2("3.3.3.3:3"); code != http.StatusTooManyRequests {
		t.Fatalf("expected capacity refusal, got %d", code)
	}
}

func TestPublicReadClientKeyUsesRemoteAddrNotXForwardedFor(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:4444"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if got := publicReadClientKey(c); got != "203.0.113.9:4444" {
		t.Fatalf("expected peer RemoteAddr, got %q", got)
	}
}
