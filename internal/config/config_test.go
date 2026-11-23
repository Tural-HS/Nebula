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
