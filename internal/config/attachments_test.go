package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAttachmentDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Attachments
	if a.Backend != "local" || a.Dir != "/var/lib/nebula/attachments" || a.MaxFileBytes != 20<<20 ||
		a.WorkspaceQuotaBytes != 5120<<20 || a.MaxPerItem != 100 || a.UploadsPerMinute != 30 ||
		a.TransferTimeout != 5*time.Minute || a.PurgeDelay != 7*24*time.Hour || !a.Enabled() || !a.S3.PathStyle {
		t.Fatalf("defaults: %+v", a)
	}
}

func TestAttachmentS3FromFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, v string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cfg, err := Load(env(map[string]string{
		"NEBULA_ATTACHMENTS_BACKEND":            "s3",
		"NEBULA_ATTACHMENTS_S3_ENDPOINT":        "https://s3.example.test",
		"NEBULA_ATTACHMENTS_S3_BUCKET":          "nebula",
		"NEBULA_ATTACHMENTS_S3_ACCESS_KEY_FILE": write("ak", "access"),
		"NEBULA_ATTACHMENTS_S3_SECRET_KEY_FILE": write("sk", "se cret"),
		"NEBULA_ATTACHMENTS_S3_PATH_STYLE":      "false",
		"NEBULA_ATTACHMENTS_WORKSPACE_QUOTA_MB": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Attachments.S3
	if s.AccessKey != "access" || s.SecretKey != "se cret" || s.PathStyle || cfg.Attachments.WorkspaceQuotaBytes != 0 {
		t.Fatalf("s3: %+v", cfg.Attachments)
	}
}

func TestAttachmentInvalid(t *testing.T) {
	for name, c := range map[string]struct {
		vars map[string]string
		want string
	}{
		"backend":       {map[string]string{"NEBULA_ATTACHMENTS_BACKEND": "ftp"}, "NEBULA_ATTACHMENTS_BACKEND"},
		"size":          {map[string]string{"NEBULA_ATTACHMENTS_MAX_FILE_MB": "0"}, "NEBULA_ATTACHMENTS_MAX_FILE_MB"},
		"quota":         {map[string]string{"NEBULA_ATTACHMENTS_WORKSPACE_QUOTA_MB": "5"}, "at least NEBULA_ATTACHMENTS_MAX_FILE_MB"},
		"s3 incomplete": {map[string]string{"NEBULA_ATTACHMENTS_BACKEND": "s3", "NEBULA_ATTACHMENTS_S3_ENDPOINT": "https://s3.example.test"}, "_SECRET_KEY"},
		"s3 http in production": {map[string]string{
			"NEBULA_ATTACHMENTS_BACKEND": "s3", "NEBULA_ATTACHMENTS_S3_ENDPOINT": "http://s3.example.test",
			"NEBULA_ATTACHMENTS_S3_BUCKET": "b", "NEBULA_ATTACHMENTS_S3_ACCESS_KEY": "a", "NEBULA_ATTACHMENTS_S3_SECRET_KEY": "s",
		}, "https"},
		"purge delay": {map[string]string{"NEBULA_ATTACHMENTS_PURGE_DELAY": "-1h"}, "NEBULA_ATTACHMENTS_PURGE_DELAY"},
		"timeout":     {map[string]string{"NEBULA_ATTACHMENTS_TRANSFER_TIMEOUT": "soon"}, "NEBULA_ATTACHMENTS_TRANSFER_TIMEOUT"},
	} {
		_, err := Load(env(c.vars))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The secret never appears in errors.
	_, err := Load(env(map[string]string{"NEBULA_ATTACHMENTS_BACKEND": "s3", "NEBULA_ATTACHMENTS_S3_SECRET_KEY": "hunter2-secret"}))
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error: %v", err)
	}
}
