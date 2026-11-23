package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunVersionNeedsNoConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"version"}, func(string) string { return "" }, &out, &errOut)
	if code != 0 || strings.TrimSpace(out.String()) != "dev" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"serve"}, func(string) string { return "" }, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "NEBULA_DATABASE_URL is required") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

func TestServeRequiresAuthConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	getenv := func(k string) string {
		return map[string]string{"NEBULA_DATABASE_URL": "postgres://u:p@localhost/db"}[k]
	}
	code := run(context.Background(), []string{"serve"}, getenv, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "NEBULA_OIDC_ISSUER is required") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	getenv := func(k string) string {
		return map[string]string{
			"NEBULA_DATABASE_URL":  "postgres://u:p@localhost/db",
			"NEBULA_OIDC_ISSUER":   "https://idp.example.com",
			"NEBULA_OIDC_AUDIENCE": "nebula-api",
		}[k]
	}
	if code := run(context.Background(), []string{"explode"}, getenv, &out, &errOut); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
