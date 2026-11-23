package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func newTest(rps float64, burst int) (*Limiter, *clock) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	l := New(rps, burst)
	l.now = c.now
	return l, c
}

func TestBurstThenRefill(t *testing.T) {
	l, c := newTest(1, 3)
	for i := range 3 {
		if ok, _ := l.Allow("u"); !ok {
			t.Fatalf("request %d within burst denied", i)
		}
	}
	ok, wait := l.Allow("u")
	if ok || wait <= 0 || wait > time.Second {
		t.Fatalf("4th request: ok=%v wait=%v", ok, wait)
	}
	// A denied request must not push the next token further out.
	if _, wait2 := l.Allow("u"); wait2 > wait {
		t.Fatalf("denied requests consumed tokens: %v > %v", wait2, wait)
	}
	c.add(time.Second)
	if ok, _ := l.Allow("u"); !ok {
		t.Fatal("token not refilled after 1s")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTest(1, 1)
	l.Allow("a")
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("a should be limited")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("b must not be affected by a")
	}
}

func TestIdleBucketsAreSweptAndMemoryIsBounded(t *testing.T) {
	l, c := newTest(1, 1)
	l.Allow("old")
	c.add(idleTTL + sweepInterval + time.Second)
	l.Allow("new")
	if _, ok := l.buckets["old"]; ok {
		t.Fatal("idle bucket not swept")
	}

	l.maxKeys = 3
	for _, k := range []string{"k1", "k2", "k3", "k4"} {
		l.Allow(k)
	}
	if len(l.buckets) > 3 {
		t.Fatalf("buckets grew to %d", len(l.buckets))
	}
}

func TestNilLimiterAllowsEverything(t *testing.T) {
	l := New(0, 10)
	if l != nil {
		t.Fatal("rps 0 must disable")
	}
	if ok, _ := l.Allow("x"); !ok {
		t.Fatal("nil limiter denied")
	}
}

func TestConcurrentUseIsSafe(_ *testing.T) {
	l := New(1000, 1000)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for range 100 {
				l.Allow(string(rune('a' + i%5)))
			}
		})
	}
	wg.Wait()
}

func TestMiddleware(t *testing.T) {
	l, _ := newTest(1, 1)
	writeErr := func(w http.ResponseWriter, status int, code, _ string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(code))
	}
	h := Middleware(l, ClientIP, writeErr)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.7:51234"
		req.Header.Set("X-Forwarded-For", "198.51.100.1") // must be ignored
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(); rec.Code != http.StatusNoContent {
		t.Fatalf("first: %d", rec.Code)
	}
	rec := call()
	if rec.Code != http.StatusTooManyRequests || rec.Body.String() != "rate_limited" || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("second: %d %q retry-after=%q", rec.Code, rec.Body.String(), rec.Header().Get("Retry-After"))
	}

	// Disabled limiter returns the handler unchanged.
	if got := Middleware(nil, ClientIP, writeErr)(h); got == nil {
		t.Fatal("nil middleware")
	}
}

func TestClientIP(t *testing.T) {
	for addr, want := range map[string]string{"203.0.113.7:1": "203.0.113.7", "[2001:db8::1]:443": "2001:db8::1", "weird": "weird"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		r.RemoteAddr = addr
		if got := ClientIP(r); got != want {
			t.Errorf("%s: got %s want %s", addr, got, want)
		}
	}
}
