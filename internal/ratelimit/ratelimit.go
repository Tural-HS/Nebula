// Package ratelimit provides in-memory, per-key token-bucket rate limiting.
//
// Limits are per process: with N replicas the effective limit is N times the
// configured one. That is acceptable as abuse protection; a shared (Redis)
// limiter can replace this behind the same middleware later.
package ratelimit

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultMaxKeys = 100_000
	idleTTL        = 10 * time.Minute
	sweepInterval  = time.Minute
)

// Limiter holds one token bucket per key. A nil *Limiter allows everything.
type Limiter struct {
	limit rate.Limit
	burst int

	mu        sync.Mutex
	buckets   map[string]*bucket
	maxKeys   int
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// New returns a limiter allowing rps requests per second with the given burst
// per key, or nil (disabled) when rps <= 0.
func New(rps float64, burst int) *Limiter {
	if rps <= 0 {
		return nil
	}
	return &Limiter{
		limit:   rate.Limit(rps),
		burst:   max(burst, 1),
		buckets: make(map[string]*bucket),
		maxKeys: defaultMaxKeys,
		now:     time.Now,
	}
}

// Allow consumes one token for key. When denied it returns how long to wait.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	now := l.now()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			// Bounded memory under key-flooding: start over rather than grow.
			clear(l.buckets)
		}
		b = &bucket{lim: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = now
	l.mu.Unlock()

	r := b.lim.ReserveN(now, 1)
	if !r.OK() {
		return false, time.Second
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now) // a denied request must not consume future tokens
		return false, d
	}
	return true, 0
}

// sweep drops idle buckets; callers hold l.mu.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.lastSeen) > idleTTL {
			delete(l.buckets, k)
		}
	}
}

// ErrorWriter writes an API error response.
type ErrorWriter func(w http.ResponseWriter, status int, code, message string)

// Middleware rejects requests over the limit with 429 and a Retry-After header.
// An empty key skips limiting for that request.
func Middleware(l *Limiter, key func(*http.Request) string, writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if l == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if k := key(r); k != "" {
				if ok, wait := l.Allow(k); !ok {
					w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
					writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many requests; retry later")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP returns the TCP peer address. Behind a reverse proxy this is the
// proxy's address, so per-IP limits should then be enforced at the ingress.
// X-Forwarded-For is deliberately ignored: clients can forge it.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
