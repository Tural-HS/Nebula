package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSecret(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const testKey = "w6M0ukXHWNDX5PY1ojFtgzK1PZiVn8v+ao5Slf/GhHM="

func TestSecretFilesAreRead(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"NEBULA_DATABASE_URL":             "",
		"NEBULA_DATABASE_URL_FILE":        writeSecret(t, "postgres://app:s3cret@db:5432/nebula\n"),
		"NEBULA_ENCRYPTION_KEY_FILE":      writeSecret(t, testKey+"\n"),
		"NEBULA_ENCRYPTION_KEYS_OLD_FILE": writeSecret(t, ""),
		"NEBULA_SMTP_PASSWORD_FILE":       writeSecret(t, " pass with spaces \r\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://app:s3cret@db:5432/nebula" || cfg.MigrateDatabaseURL != cfg.DatabaseURL {
		t.Errorf("database urls: %q %q", cfg.DatabaseURL, cfg.MigrateDatabaseURL)
	}
	if cfg.EncryptionKey != testKey || len(cfg.OldEncryptionKeys) != 0 {
		t.Errorf("keys: %q %v", cfg.EncryptionKey, cfg.OldEncryptionKeys)
	}
	// Exactly one trailing line break is removed; the rest is the secret.
	if cfg.SMTP.Password != " pass with spaces " {
		t.Errorf("smtp password: %q", cfg.SMTP.Password)
	}
	if err := cfg.ValidateRuntime(); err != nil {
		t.Errorf("runtime: %v", err)
	}
}

func TestSecretFileTrimsOnlyOneNewline(t *testing.T) {
	cfg, err := Load(env(map[string]string{"NEBULA_SMTP_PASSWORD_FILE": writeSecret(t, "pw\n\n")}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTP.Password != "pw\n" {
		t.Errorf("got %q", cfg.SMTP.Password)
	}
}

func TestSecretFileErrors(t *testing.T) {
	dir := t.TempDir()
	unreadable := writeSecret(t, "x")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	big := writeSecret(t, strings.Repeat("a", maxSecretFileSize+1))
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"both set", map[string]string{"NEBULA_ENCRYPTION_KEY": testKey, "NEBULA_ENCRYPTION_KEY_FILE": writeSecret(t, testKey)},
			"NEBULA_ENCRYPTION_KEY and NEBULA_ENCRYPTION_KEY_FILE are both set"},
		{"missing file", map[string]string{"NEBULA_SMTP_PASSWORD_FILE": filepath.Join(dir, "nope")},
			"NEBULA_SMTP_PASSWORD_FILE: cannot read secret file"},
		{"directory", map[string]string{"NEBULA_ENCRYPTION_KEYS_OLD_FILE": dir}, "not a regular file"},
		{"too large", map[string]string{"NEBULA_DATABASE_PASSWORD_FILE": big}, "larger than"},
	}
	if os.Geteuid() != 0 { // root reads files regardless of mode
		tests = append(tests, struct {
			name    string
			env     map[string]string
			wantErr string
		}{"unreadable", map[string]string{"NEBULA_ENCRYPTION_KEY_FILE": unreadable}, "permission denied"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
			if strings.Contains(err.Error(), testKey) {
				t.Fatalf("error leaks the secret: %v", err)
			}
		})
	}
}

func TestDatabasePasswordAndMigrateURL(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"NEBULA_DATABASE_URL":                   "postgres://nebula_app@db:5432/nebula?sslmode=disable",
		"NEBULA_DATABASE_PASSWORD_FILE":         writeSecret(t, "p@ss/w:rd\n"),
		"NEBULA_MIGRATE_DATABASE_URL":           "postgres://nebula@db:5432/nebula?sslmode=disable",
		"NEBULA_MIGRATE_DATABASE_PASSWORD_FILE": writeSecret(t, "owner-pw"),
		"NEBULA_DB_RUNTIME_ROLE":                "nebula_app",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if want := "postgres://nebula_app:p%40ss%2Fw%3Ard@db:5432/nebula?sslmode=disable"; cfg.DatabaseURL != want {
		t.Errorf("runtime url %q, want %q", cfg.DatabaseURL, want)
	}
	if want := "postgres://nebula:owner-pw@db:5432/nebula?sslmode=disable"; cfg.MigrateDatabaseURL != want {
		t.Errorf("migrate url %q, want %q", cfg.MigrateDatabaseURL, want)
	}
	if cfg.DBRuntimeRole != "nebula_app" {
		t.Errorf("runtime role %q", cfg.DBRuntimeRole)
	}
}

func TestMigrateOnlyConfig(t *testing.T) {
	// The migrate container gets only the owner connection.
	cfg, err := Load(env(map[string]string{
		"NEBULA_DATABASE_URL":         "",
		"NEBULA_MIGRATE_DATABASE_URL": "postgres://nebula:pw@db/nebula",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MigrateDatabaseURL == "" || cfg.DatabaseURL != "" {
		t.Fatalf("urls: %+v", cfg)
	}
	cfg.EncryptionKey = testKey
	if err := cfg.ValidateRuntime(); err == nil || !strings.Contains(err.Error(), "NEBULA_DATABASE_URL is required") {
		t.Fatalf("serve must need the runtime url, got %v", err)
	}
}

func TestDatabaseSettingsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"password twice", map[string]string{"NEBULA_DATABASE_PASSWORD": "x"}, "already contains a password"},
		{"password without user", map[string]string{"NEBULA_DATABASE_URL": "postgres://db/nebula", "NEBULA_DATABASE_PASSWORD": "x"}, "must contain a user name"},
		{"migrate password alone", map[string]string{"NEBULA_MIGRATE_DATABASE_PASSWORD": "x"}, "NEBULA_MIGRATE_DATABASE_URL is not"},
		{"bad migrate url", map[string]string{"NEBULA_MIGRATE_DATABASE_URL": "mysql://u:secretpw@x"}, "NEBULA_MIGRATE_DATABASE_URL must be a postgres:// URL"},
		{"bad role", map[string]string{"NEBULA_DB_RUNTIME_ROLE": `app"; DROP`}, "NEBULA_DB_RUNTIME_ROLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
			if strings.Contains(err.Error(), "secretpw") {
				t.Fatalf("error leaks the password: %v", err)
			}
		})
	}
}
