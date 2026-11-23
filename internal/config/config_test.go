package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// env returns a getenv over m plus valid defaults for required settings,
// so each test only states what it is about. A "" value unsets a default.
func env(m map[string]string) func(string) string {
	base := map[string]string{
		"NEBULA_DATABASE_URL":  "postgres://u:p@localhost:5432/nebula",
		"NEBULA_OIDC_ISSUER":   "https://idp.example.com/realms/nebula",
		"NEBULA_OIDC_AUDIENCE": "nebula-api",
	}
	for k, v := range m {
		base[k] = v
	}
	return func(k string) string { return base[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != "production" || cfg.HTTPAddr != ":8080" || cfg.AdminAddr != ":9090" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.DBMaxConns != 20 || cfg.ShutdownTimeout != 25*time.Second {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.RateLimit != (RateLimit{UserRPS: 20, UserBurst: 40, IPRPS: 100, IPBurst: 200}) {
		t.Errorf("rate limit defaults: %+v", cfg.RateLimit)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing db url", map[string]string{"NEBULA_DATABASE_URL": ""}, "NEBULA_DATABASE_URL is required"},
		{"bad scheme", map[string]string{"NEBULA_DATABASE_URL": "mysql://x"}, "must be a postgres:// URL"},
		{"bad env", map[string]string{"NEBULA_DATABASE_URL": "postgres://x", "NEBULA_ENV": "staging"}, "NEBULA_ENV"},
		{"bad level", map[string]string{"NEBULA_DATABASE_URL": "postgres://x", "NEBULA_LOG_LEVEL": "loud"}, "NEBULA_LOG_LEVEL"},
		{"bad conns", map[string]string{"NEBULA_DATABASE_URL": "postgres://x", "NEBULA_DB_MAX_CONNS": "0"}, "NEBULA_DB_MAX_CONNS"},
		{"bad timeout", map[string]string{"NEBULA_DATABASE_URL": "postgres://x", "NEBULA_SHUTDOWN_TIMEOUT": "-1s"}, "NEBULA_SHUTDOWN_TIMEOUT"},
		{"same ports", map[string]string{"NEBULA_DATABASE_URL": "postgres://x", "NEBULA_ADMIN_ADDR": ":8080"}, "must differ"},
		{"bad addr", map[string]string{"NEBULA_DATABASE_URL": "postgres://x", "NEBULA_HTTP_ADDR": "8080"}, "NEBULA_HTTP_ADDR"},
		{"negative rps", map[string]string{"NEBULA_RATE_LIMIT_USER_RPS": "-1"}, "NEBULA_RATE_LIMIT_USER_RPS"},
		{"nan rps", map[string]string{"NEBULA_RATE_LIMIT_IP_RPS": "NaN"}, "NEBULA_RATE_LIMIT_IP_RPS"},
		{"zero burst", map[string]string{"NEBULA_RATE_LIMIT_USER_BURST": "0"}, "NEBULA_RATE_LIMIT_USER_BURST"},
		{"bad proxy cidr", map[string]string{"NEBULA_TRUSTED_PROXIES": "10.0.0.0/8,10.0.0.0/40"}, "NEBULA_TRUSTED_PROXIES"},
		{"proxy host bits", map[string]string{"NEBULA_TRUSTED_PROXIES": "10.1.2.3/8"}, "host bits"},
		{"relative public url", map[string]string{"NEBULA_PUBLIC_URL": "nebula.example.com"}, "NEBULA_PUBLIC_URL"},
		{"http public url in prod", map[string]string{"NEBULA_PUBLIC_URL": "http://nebula.example.com"}, "must use https"},
		{"public url with query", map[string]string{"NEBULA_PUBLIC_URL": "https://nebula.example.com/?a=1"}, "query"},
		{"public url with credentials", map[string]string{"NEBULA_PUBLIC_URL": "https://u:p@nebula.example.com"}, "credentials"},
		{"javascript public url", map[string]string{"NEBULA_PUBLIC_URL": "javascript:alert(1)"}, "absolute http(s)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(env(tt.env))
			if err == nil {
				err = cfg.ValidateAuth()
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateAuth(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing issuer", map[string]string{"NEBULA_OIDC_ISSUER": ""}, "NEBULA_OIDC_ISSUER is required"},
		{"missing audience", map[string]string{"NEBULA_OIDC_AUDIENCE": ""}, "NEBULA_OIDC_AUDIENCE is required"},
		{"relative issuer", map[string]string{"NEBULA_OIDC_ISSUER": "/realms/x"}, "absolute http(s) URL"},
		{"http issuer in prod", map[string]string{"NEBULA_OIDC_ISSUER": "http://idp/realms/x"}, "https in production"},
		{"http jwks in prod", map[string]string{"NEBULA_OIDC_JWKS_URL": "http://idp/certs"}, "https in production"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(env(tt.env))
			if err != nil {
				t.Fatalf("auth settings must not fail Load: %v", err)
			}
			if err := cfg.ValidateAuth(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadAllowsHTTPIdPInDevelopment(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"NEBULA_ENV":           "development",
		"NEBULA_OIDC_ISSUER":   "http://localhost:8081/realms/nebula",
		"NEBULA_OIDC_JWKS_URL": "http://keycloak:8080/realms/nebula/protocol/openid-connect/certs",
	}))
	if err == nil {
		err = cfg.ValidateAuth()
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadNeverLeaksPassword(t *testing.T) {
	_, err := Load(env(map[string]string{"NEBULA_DATABASE_URL": "mysql://user:s3cret@host/db"}))
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error must not contain the password: %v", err)
	}
}

func TestPhase1SecurityRules(t *testing.T) {
	prodErrors := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"smtp without tls in prod", map[string]string{"NEBULA_SMTP_HOST": "smtp.example.com", "NEBULA_SMTP_FROM": "a@example.com", "NEBULA_SMTP_TLS": "none"}, "not allowed in production"},
		{"smtp without from", map[string]string{"NEBULA_SMTP_HOST": "smtp.example.com"}, "NEBULA_SMTP_FROM is required"},
		{"bad smtp tls", map[string]string{"NEBULA_SMTP_HOST": "smtp.example.com", "NEBULA_SMTP_FROM": "a@example.com", "NEBULA_SMTP_TLS": "ssl"}, "NEBULA_SMTP_TLS"},
		{"private webhooks in prod", map[string]string{"NEBULA_WEBHOOK_ALLOW_PRIVATE": "true"}, "SSRF"},
		{"bad worker count", map[string]string{"NEBULA_WORKER_CONCURRENCY": "-1"}, "NEBULA_WORKER_CONCURRENCY"},
	}
	for _, tt := range prodErrors {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(env(tt.env)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %q, got %v", tt.want, err)
			}
		})
	}
	dev, err := Load(env(map[string]string{"NEBULA_ENV": "development", "NEBULA_WEBHOOK_ALLOW_PRIVATE": "true",
		"NEBULA_SMTP_HOST": "mailpit", "NEBULA_SMTP_FROM": "a@localhost.test", "NEBULA_SMTP_TLS": "none"}))
	if err != nil || !dev.Webhooks.AllowPrivate {
		t.Fatalf("development must allow private webhooks and plain SMTP: %v", err)
	}
}

func TestValidateRuntimeEncryptionKey(t *testing.T) {
	for key, want := range map[string]string{
		"":            "NEBULA_ENCRYPTION_KEY is required",
		"not-base64!": "32 bytes",
		"c2hvcnQ=":    "32 bytes", // "short"
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=": "",
	} {
		cfg, err := Load(env(map[string]string{"NEBULA_ENCRYPTION_KEY": key}))
		if err != nil {
			t.Fatal(err)
		}
		err = cfg.ValidateRuntime()
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("key %q: want %q, got %v", key, want, err)
		}
	}
}

func TestOldEncryptionKeys(t *testing.T) {
	cur := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	old := "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	for list, want := range map[string]string{
		old + " , " + old: "",
		"short":           "entry 1",
		old + ",bad":      "entry 2",
		cur:               "must not contain the current",
	} {
		cfg, err := Load(env(map[string]string{"NEBULA_ENCRYPTION_KEY": cur, "NEBULA_ENCRYPTION_KEYS_OLD": list}))
		if err != nil {
			t.Fatal(err)
		}
		err = cfg.ValidateRuntime()
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: want %q, got %v", list, want, err)
		}
	}
}

func TestLoadTrustedProxiesAndPublicURL(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil || len(cfg.TrustedProxies) != 0 || cfg.PublicURL != "" {
		t.Fatalf("defaults: %v %v %q", err, cfg.TrustedProxies, cfg.PublicURL)
	}
	cfg, err = Load(env(map[string]string{
		"NEBULA_TRUSTED_PROXIES": " 10.0.0.0/8, fd00::/8 ,192.0.2.1",
		"NEBULA_PUBLIC_URL":      "https://nebula.example.com/app/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 3 || cfg.TrustedProxies[2].String() != "192.0.2.1/32" {
		t.Errorf("proxies: %v", cfg.TrustedProxies)
	}
	if cfg.PublicURL != "https://nebula.example.com/app" {
		t.Errorf("public url: %q", cfg.PublicURL)
	}
	cfg, err = Load(env(map[string]string{"NEBULA_ENV": "development", "NEBULA_PUBLIC_URL": "http://localhost:5173"}))
	if err != nil || cfg.PublicURL != "http://localhost:5173" {
		t.Fatalf("dev http: %v %q", err, cfg.PublicURL)
	}
}
