// Package config loads and validates runtime configuration from environment
// variables. Invalid configuration fails fast at startup, never at request time.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Tural-HS/Nebula/internal/ratelimit"
)

// Config is the complete runtime configuration of the service.
type Config struct {
	Env         string // "development" or "production"
	HTTPAddr    string // public API listener
	AdminAddr   string // metrics listener; never expose publicly
	DatabaseURL string // runtime connection (serve, worker, rotate-secrets)
	// MigrateDatabaseURL is used only by "migrate": the schema owner. Defaults
	// to DatabaseURL (single-role setups such as the dev stack).
	MigrateDatabaseURL string
	// DBRuntimeRole, when set, is granted DML (no DDL) on every table and
	// sequence by "migrate up", including future ones via default privileges.
	DBRuntimeRole   string
	DBMaxConns      int32
	LogLevel        slog.Level
	LogFormat       string // "json" or "text"
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
	OIDCIssuer      string // must equal the token "iss" claim exactly
	OIDCAudience    string // must appear in the token "aud" claim
	OIDCJWKSURL     string // optional; skips discovery (internal IdP address)
	RateLimit       RateLimit
	// TrustedProxies are reverse proxies whose X-Forwarded-For is believed when
	// resolving the client IP for rate limiting. Empty: use the TCP peer only.
	TrustedProxies []netip.Prefix
	// PublicURL is the browser-facing base URL of the web UI (no trailing
	// slash), used for links in chat notifications. Empty: no links.
	PublicURL         string
	EncryptionKey     string   // base64 32 bytes; encrypts webhook secrets at rest
	OldEncryptionKeys []string // retired keys, decrypt-only, during rotation
	SMTP              SMTP
	Webhooks          Webhooks
	WorkerConcurrency int // background jobs in this process; 0 disables
	// JobMetricsInterval is how often the job-queue gauges are refreshed from
	// the database (one cheap query per process and interval, not per scrape).
	JobMetricsInterval time.Duration
	Attachments        Attachments
}

// SMTP configures outgoing email. Empty Host disables email.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      string // starttls | tls | none
}

// Webhooks configures outgoing webhook delivery.
type Webhooks struct {
	AllowPrivate bool // allow loopback/private targets; development only
}

// RateLimit configures in-memory request limits (per replica). RPS 0 disables.
type RateLimit struct {
	UserRPS   float64 // per authenticated user
	UserBurst int
	IPRPS     float64 // per client IP, before authentication
	IPBurst   int
}

// Load reads configuration through getenv (os.Getenv in production, a map in
// tests). Every key in SecretKeys may instead be read from the file named by
// KEY_FILE.
func Load(rawGetenv func(string) string) (Config, error) {
	getenv, errs := withSecretFiles(rawGetenv)
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := Config{
		Env:          get("NEBULA_ENV", "production"),
		HTTPAddr:     get("NEBULA_HTTP_ADDR", ":8080"),
		AdminAddr:    get("NEBULA_ADMIN_ADDR", ":9090"),
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

	errs = append(errs, cfg.loadDatabase(get, getenv)...)

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

	proxies, err := ratelimit.ParseTrustedProxies(get("NEBULA_TRUSTED_PROXIES", ""))
	if err != nil {
		errs = append(errs, fmt.Errorf("NEBULA_TRUSTED_PROXIES: %w", err))
	}
	cfg.TrustedProxies = proxies

	if raw := get("NEBULA_PUBLIC_URL", ""); raw != "" {
		u, err := checkPublicURL(raw, cfg.Env)
		if err != nil {
			errs = append(errs, fmt.Errorf("NEBULA_PUBLIC_URL %w", err))
		}
		cfg.PublicURL = u
	}

	cfg.EncryptionKey = get("NEBULA_ENCRYPTION_KEY", "")
	for k := range strings.SplitSeq(get("NEBULA_ENCRYPTION_KEYS_OLD", ""), ",") {
		if k = strings.TrimSpace(k); k != "" {
			cfg.OldEncryptionKeys = append(cfg.OldEncryptionKeys, k)
		}
	}
	cfg.SMTP = SMTP{
		Host:     get("NEBULA_SMTP_HOST", ""),
		Username: get("NEBULA_SMTP_USERNAME", ""),
		Password: getenv("NEBULA_SMTP_PASSWORD"), // not trimmed: may contain spaces
		From:     get("NEBULA_SMTP_FROM", ""),
		TLS:      get("NEBULA_SMTP_TLS", "starttls"),
	}
	port, err := strconv.Atoi(get("NEBULA_SMTP_PORT", "587"))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, errors.New("NEBULA_SMTP_PORT must be a port number"))
	}
	cfg.SMTP.Port = port
	if cfg.SMTP.Host != "" {
		if cfg.SMTP.From == "" {
			errs = append(errs, errors.New("NEBULA_SMTP_FROM is required when NEBULA_SMTP_HOST is set"))
		}
		switch cfg.SMTP.TLS {
		case "starttls", "tls":
		case "none":
			if cfg.Env == "production" {
				errs = append(errs, errors.New("NEBULA_SMTP_TLS=none is not allowed in production"))
			}
		default:
			errs = append(errs, errors.New("NEBULA_SMTP_TLS must be starttls, tls or none"))
		}
	}
	allowPrivate, err := strconv.ParseBool(get("NEBULA_WEBHOOK_ALLOW_PRIVATE", "false"))
	if err != nil {
		errs = append(errs, errors.New("NEBULA_WEBHOOK_ALLOW_PRIVATE must be true or false"))
	}
	if allowPrivate && cfg.Env == "production" {
		errs = append(errs, errors.New("NEBULA_WEBHOOK_ALLOW_PRIVATE=true is not allowed in production (SSRF risk)"))
	}
	cfg.Webhooks.AllowPrivate = allowPrivate
	workers, err := strconv.Atoi(get("NEBULA_WORKER_CONCURRENCY", "4"))
	if err != nil || workers < 0 || workers > 256 {
		errs = append(errs, errors.New("NEBULA_WORKER_CONCURRENCY must be an integer between 0 and 256"))
	}
	cfg.WorkerConcurrency = workers

	cfg.Attachments, err = loadAttachments(get, getenv, cfg.Env)
	if err != nil {
		errs = append(errs, err)
	}

	cfg.ShutdownTimeout = parseDuration(get("NEBULA_SHUTDOWN_TIMEOUT", "25s"), "NEBULA_SHUTDOWN_TIMEOUT", &errs)
	cfg.RequestTimeout = parseDuration(get("NEBULA_REQUEST_TIMEOUT", "30s"), "NEBULA_REQUEST_TIMEOUT", &errs)
	cfg.JobMetricsInterval = parseDuration(get("NEBULA_JOB_METRICS_INTERVAL", "30s"), "NEBULA_JOB_METRICS_INTERVAL", &errs)
	if d := cfg.JobMetricsInterval; d > 0 && (d < 5*time.Second || d > 10*time.Minute) {
		errs = append(errs, errors.New("NEBULA_JOB_METRICS_INTERVAL must be between 5s and 10m"))
	}

	return cfg, errors.Join(errs...)
}

// ValidateRuntime checks settings needed by serve and worker (not migrate).
func (c Config) ValidateRuntime() error {
	if c.DatabaseURL == "" {
		// Only NEBULA_MIGRATE_DATABASE_URL is set: enough for migrate, not for the app.
		return errors.New("NEBULA_DATABASE_URL is required")
	}
	key, err := base64.StdEncoding.DecodeString(c.EncryptionKey)
	switch {
	case c.EncryptionKey == "":
		return errors.New("NEBULA_ENCRYPTION_KEY is required (generate one with: openssl rand -base64 32)")
	case err != nil || len(key) != 32:
		return errors.New("NEBULA_ENCRYPTION_KEY must be 32 bytes, base64 encoded (openssl rand -base64 32)")
	}
	for i, k := range c.OldEncryptionKeys {
		if b, err := base64.StdEncoding.DecodeString(k); err != nil || len(b) != 32 {
			return fmt.Errorf("NEBULA_ENCRYPTION_KEYS_OLD entry %d must be 32 bytes, base64 encoded", i+1)
		}
		if k == c.EncryptionKey {
			return errors.New("NEBULA_ENCRYPTION_KEYS_OLD must not contain the current NEBULA_ENCRYPTION_KEY")
		}
	}
	return nil
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

// checkPublicURL requires an absolute https URL without query, fragment or
// credentials (http only in development) and returns it without a trailing slash.
func checkPublicURL(raw, env string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("must be an absolute http(s) URL")
	}
	if u.Scheme != "https" && env != "development" {
		return "", errors.New("must use https (http is allowed only with NEBULA_ENV=development)")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("must not contain credentials, a query or a fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// loadDatabase reads the runtime and migration connection settings. Passwords
// are not trimmed: they may legitimately contain spaces.
func (c *Config) loadDatabase(get func(key, def string) string, getenv func(string) string) []error {
	var errs []error
	runtimeURL, migrateURL := get("NEBULA_DATABASE_URL", ""), get("NEBULA_MIGRATE_DATABASE_URL", "")
	runtimePW, migratePW := getenv("NEBULA_DATABASE_PASSWORD"), getenv("NEBULA_MIGRATE_DATABASE_PASSWORD")
	if runtimeURL == "" && migrateURL == "" {
		return []error{errors.New("NEBULA_DATABASE_URL is required")}
	}
	if runtimeURL != "" {
		u, err := databaseURL(runtimeURL, runtimePW, "NEBULA_DATABASE_URL", "NEBULA_DATABASE_PASSWORD")
		if err != nil {
			errs = append(errs, err)
		}
		c.DatabaseURL = u
	} else if runtimePW != "" {
		errs = append(errs, errors.New("NEBULA_DATABASE_PASSWORD is set but NEBULA_DATABASE_URL is not"))
	}
	switch {
	case migrateURL != "":
		u, err := databaseURL(migrateURL, migratePW, "NEBULA_MIGRATE_DATABASE_URL", "NEBULA_MIGRATE_DATABASE_PASSWORD")
		if err != nil {
			errs = append(errs, err)
		}
		c.MigrateDatabaseURL = u
	case migratePW != "":
		errs = append(errs, errors.New("NEBULA_MIGRATE_DATABASE_PASSWORD is set but NEBULA_MIGRATE_DATABASE_URL is not"))
	default:
		c.MigrateDatabaseURL = c.DatabaseURL
	}
	if role := get("NEBULA_DB_RUNTIME_ROLE", ""); role != "" {
		if err := checkRoleName("NEBULA_DB_RUNTIME_ROLE", role); err != nil {
			errs = append(errs, err)
		}
		c.DBRuntimeRole = role
	}
	return errs
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
