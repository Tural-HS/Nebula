// Package config loads and validates runtime configuration from environment
// variables. Invalid configuration fails fast at startup, never at request time.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config is the complete runtime configuration of the service.
type Config struct {
	Env             string // "development" or "production"
	HTTPAddr        string // public API listener
	AdminAddr       string // metrics listener; never expose publicly
	DatabaseURL     string
	DBMaxConns      int32
	LogLevel        slog.Level
	LogFormat       string // "json" or "text"
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
	OIDCIssuer      string // must equal the token "iss" claim exactly
	OIDCAudience    string // must appear in the token "aud" claim
	OIDCJWKSURL     string // optional; skips discovery (internal IdP address)
	RateLimit       RateLimit
}

// RateLimit configures in-memory request limits (per replica). RPS 0 disables.
type RateLimit struct {
	UserRPS   float64 // per authenticated user
	UserBurst int
	IPRPS     float64 // per client IP, before authentication
	IPBurst   int
}

// Load reads configuration through getenv (os.Getenv in production, a map in tests).
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	var errs []error
	cfg := Config{
		Env:          get("NEBULA_ENV", "production"),
		HTTPAddr:     get("NEBULA_HTTP_ADDR", ":8080"),
		AdminAddr:    get("NEBULA_ADMIN_ADDR", ":9090"),
		DatabaseURL:  get("NEBULA_DATABASE_URL", ""),
		LogFormat:    get("NEBULA_LOG_FORMAT", "json"),
		OIDCIssuer:   get("NEBULA_OIDC_ISSUER", ""),
		OIDCAudience: get("NEBULA_OIDC_AUDIENCE", ""),
		OIDCJWKSURL:  get("NEBULA_OIDC_JWKS_URL", ""),
	}

	if cfg.Env != "development" && cfg.Env != "production" {
		errs = append(errs, fmt.Errorf("NEBULA_ENV must be development or production, got %q", cfg.Env))
	}
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		errs = append(errs, fmt.Errorf("NEBULA_LOG_FORMAT must be json or text, got %q", cfg.LogFormat))
	}
	for key, addr := range map[string]string{"NEBULA_HTTP_ADDR": cfg.HTTPAddr, "NEBULA_ADMIN_ADDR": cfg.AdminAddr} {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			errs = append(errs, fmt.Errorf("%s is not a valid host:port: %w", key, err))
		}
	}
	if cfg.HTTPAddr == cfg.AdminAddr {
		errs = append(errs, errors.New("NEBULA_HTTP_ADDR and NEBULA_ADMIN_ADDR must differ"))
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("NEBULA_DATABASE_URL is required"))
	} else if u, err := url.Parse(cfg.DatabaseURL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		// Never echo the URL: it contains the password.
		errs = append(errs, errors.New("NEBULA_DATABASE_URL must be a postgres:// URL"))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(get("NEBULA_LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("NEBULA_LOG_LEVEL: %w", err))
	}

	maxConns, err := strconv.ParseInt(get("NEBULA_DB_MAX_CONNS", "20"), 10, 32)
	if err != nil || maxConns < 1 || maxConns > 500 {
		errs = append(errs, errors.New("NEBULA_DB_MAX_CONNS must be an integer between 1 and 500"))
	}
	cfg.DBMaxConns = int32(maxConns)

	cfg.RateLimit = RateLimit{
		UserRPS:   parseRate(get("NEBULA_RATE_LIMIT_USER_RPS", "20"), "NEBULA_RATE_LIMIT_USER_RPS", &errs),
		UserBurst: parseBurst(get("NEBULA_RATE_LIMIT_USER_BURST", "40"), "NEBULA_RATE_LIMIT_USER_BURST", &errs),
		IPRPS:     parseRate(get("NEBULA_RATE_LIMIT_IP_RPS", "100"), "NEBULA_RATE_LIMIT_IP_RPS", &errs),
		IPBurst:   parseBurst(get("NEBULA_RATE_LIMIT_IP_BURST", "200"), "NEBULA_RATE_LIMIT_IP_BURST", &errs),
	}

	cfg.ShutdownTimeout = parseDuration(get("NEBULA_SHUTDOWN_TIMEOUT", "25s"), "NEBULA_SHUTDOWN_TIMEOUT", &errs)
	cfg.RequestTimeout = parseDuration(get("NEBULA_REQUEST_TIMEOUT", "30s"), "NEBULA_REQUEST_TIMEOUT", &errs)

	return cfg, errors.Join(errs...)
}

// ValidateAuth checks the settings only the API server needs, so operational
// commands like "migrate" don't require identity provider configuration.
func (c Config) ValidateAuth() error {
	var errs []error
	if c.OIDCIssuer == "" {
		errs = append(errs, errors.New("NEBULA_OIDC_ISSUER is required"))
	} else if err := checkURL(c.OIDCIssuer, c.Env); err != nil {
		errs = append(errs, fmt.Errorf("NEBULA_OIDC_ISSUER %w", err))
	}
	if c.OIDCAudience == "" {
		errs = append(errs, errors.New("NEBULA_OIDC_AUDIENCE is required"))
	}
	if c.OIDCJWKSURL != "" {
		if err := checkURL(c.OIDCJWKSURL, c.Env); err != nil {
			errs = append(errs, fmt.Errorf("NEBULA_OIDC_JWKS_URL %w", err))
		}
	}
	return errors.Join(errs...)
}

// checkURL requires an absolute URL; production additionally requires https,
// because token keys fetched over plain HTTP could be swapped by an attacker.
func checkURL(raw, env string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("must be an absolute http(s) URL")
	}
	if env == "production" && u.Scheme != "https" {
		return errors.New("must use https in production")
	}
	return nil
}

func parseRate(raw, key string, errs *[]error) float64 {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || f > 100_000 || f != f {
		*errs = append(*errs, fmt.Errorf("%s must be a number between 0 (disabled) and 100000", key))
	}
	return f
}

func parseBurst(raw, key string, errs *[]error) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100_000 {
		*errs = append(*errs, fmt.Errorf("%s must be an integer between 1 and 100000", key))
	}
	return n
}

func parseDuration(raw, key string, errs *[]error) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive duration like 30s", key))
	}
	return d
}
