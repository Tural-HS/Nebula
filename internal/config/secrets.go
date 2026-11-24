package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// SecretKeys are the settings that may also be given as KEY_FILE: the path of
// a file holding the value (Docker/Compose secrets, Kubernetes secret volumes,
// OpenBao Agent templates). Secrets then never appear in the process
// environment, `docker inspect` or crash dumps of the environment.
var SecretKeys = []string{
	"NEBULA_DATABASE_URL",
	"NEBULA_DATABASE_PASSWORD",
	"NEBULA_MIGRATE_DATABASE_URL",
	"NEBULA_MIGRATE_DATABASE_PASSWORD",
	"NEBULA_ENCRYPTION_KEY",
	"NEBULA_ENCRYPTION_KEYS_OLD",
	"NEBULA_SMTP_PASSWORD",
	"NEBULA_ATTACHMENTS_S3_ACCESS_KEY",
	"NEBULA_ATTACHMENTS_S3_SECRET_KEY",
	"NEBULA_EVIDENCE_SIGNING_KEY",
}

// maxSecretFileSize bounds what a *_FILE may contain: secrets are short, and a
// wrong path (a log file, /dev/zero) must not make startup read gigabytes.
const maxSecretFileSize = 64 << 10

// withSecretFiles returns a getenv that answers SecretKeys from KEY_FILE when
// it is set. Setting both KEY and KEY_FILE is an error, so it is never
// ambiguous which value is in effect. Errors name keys and paths, never values.
func withSecretFiles(getenv func(string) string) (func(string) string, []error) {
	var errs []error
	fromFile := map[string]string{}
	for _, key := range SecretKeys {
		path := strings.TrimSpace(getenv(key + "_FILE"))
		if path == "" {
			continue
		}
		if getenv(key) != "" {
			errs = append(errs, fmt.Errorf("%s and %s_FILE are both set; set only one", key, key))
			continue
		}
		v, err := readSecretFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s_FILE: %w", key, err))
		}
		fromFile[key] = v
	}
	return func(key string) string {
		if v, ok := fromFile[key]; ok {
			return v
		}
		return getenv(key)
	}, errs
}

// readSecretFile reads a regular file and removes exactly one trailing line
// break ("\n" or "\r\n"), which editors and `echo` add. Anything else, including
// other whitespace, is part of the secret.
func readSecretFile(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- the operator chooses the path
	if err != nil {
		return "", fmt.Errorf("cannot read secret file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("cannot read secret file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("secret file %s is not a regular file", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSecretFileSize+1))
	if err != nil {
		return "", fmt.Errorf("cannot read secret file: %w", err)
	}
	if len(b) > maxSecretFileSize {
		return "", fmt.Errorf("secret file %s is larger than %d bytes", path, maxSecretFileSize)
	}
	s := string(b)
	if t, ok := strings.CutSuffix(s, "\n"); ok {
		s = strings.TrimSuffix(t, "\r")
	}
	return s, nil
}

// databaseURL validates a postgres:// URL and, when password is set, puts it
// into the URL. The password lives in its own setting so the URL itself is not
// secret, and one password file can serve both Nebula and the database init.
func databaseURL(raw, password, urlKey, passwordKey string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		// Never echo the URL or the parse error (which quotes it): it may contain the password.
		return "", fmt.Errorf("%s must be a postgres:// URL", urlKey)
	}
	if password == "" {
		return raw, nil
	}
	if u.User == nil || u.User.Username() == "" {
		return "", fmt.Errorf("%s must contain a user name when %s is set", urlKey, passwordKey)
	}
	if _, has := u.User.Password(); has {
		return "", fmt.Errorf("%s already contains a password; set it either there or in %s", urlKey, passwordKey)
	}
	u.User = url.UserPassword(u.User.Username(), password)
	return u.String(), nil
}

// roleName is a plain, unquoted PostgreSQL identifier.
var roleName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

func checkRoleName(key, v string) error {
	if !roleName.MatchString(v) {
		return errors.New(key + " must be a lower-case PostgreSQL role name ([a-z_][a-z0-9_]*)")
	}
	return nil
}
