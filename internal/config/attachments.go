package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Attachments configures file attachments (ADR-0022).
type Attachments struct {
	Backend string // "local" (default), "s3" or "disabled"
	Dir     string // local backend: blob directory (a persistent volume)
	S3      S3
	// TempDir holds S3 staging files while an upload is checked. Empty: os.TempDir().
	TempDir             string
	MaxFileBytes        int64
	WorkspaceQuotaBytes int64 // 0: unlimited
	MaxPerItem          int   // 0: unlimited
	UploadsPerMinute    int   // per user and replica; 0: unlimited
	TransferTimeout     time.Duration
	// PurgeDelay keeps deleted files this long before removing them, so a
	// point-in-time database restore finds them in the file backup.
	PurgeDelay time.Duration
}

// S3 configures an S3-compatible bucket. Keys may be given as *_FILE.
type S3 struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool
	Prefix    string
}

// Enabled reports whether attachments are available.
func (a Attachments) Enabled() bool { return a.Backend != "disabled" }

func loadAttachments(get func(key, def string) string, getenv func(string) string, env string) (Attachments, error) {
	var errs []error
	a := Attachments{
		Backend: get("NEBULA_ATTACHMENTS_BACKEND", "local"),
		Dir:     get("NEBULA_ATTACHMENTS_DIR", "/var/lib/nebula/attachments"),
		TempDir: get("NEBULA_ATTACHMENTS_TMP_DIR", ""),
		S3: S3{
			Endpoint:  get("NEBULA_ATTACHMENTS_S3_ENDPOINT", ""),
			Region:    get("NEBULA_ATTACHMENTS_S3_REGION", "us-east-1"),
			Bucket:    get("NEBULA_ATTACHMENTS_S3_BUCKET", ""),
			AccessKey: get("NEBULA_ATTACHMENTS_S3_ACCESS_KEY", ""),
			SecretKey: getenv("NEBULA_ATTACHMENTS_S3_SECRET_KEY"), // not trimmed
			Prefix:    get("NEBULA_ATTACHMENTS_S3_PREFIX", ""),
		},
	}
	pathStyle, err := strconv.ParseBool(get("NEBULA_ATTACHMENTS_S3_PATH_STYLE", "true"))
	if err != nil {
		errs = append(errs, errors.New("NEBULA_ATTACHMENTS_S3_PATH_STYLE must be true or false"))
	}
	a.S3.PathStyle = pathStyle

	intIn := func(key, def string, lo, hi int64) int64 {
		n, err := strconv.ParseInt(get(key, def), 10, 64)
		if err != nil || n < lo || n > hi {
			errs = append(errs, fmt.Errorf("%s must be an integer between %d and %d", key, lo, hi))
		}
		return n
	}
	a.MaxFileBytes = intIn("NEBULA_ATTACHMENTS_MAX_FILE_MB", "20", 1, 1024) << 20
	a.WorkspaceQuotaBytes = intIn("NEBULA_ATTACHMENTS_WORKSPACE_QUOTA_MB", "5120", 0, 1<<24) << 20
	a.MaxPerItem = int(intIn("NEBULA_ATTACHMENTS_MAX_PER_ITEM", "100", 0, 10_000))
	a.UploadsPerMinute = int(intIn("NEBULA_ATTACHMENTS_UPLOADS_PER_MINUTE", "30", 0, 10_000))
	a.TransferTimeout = parseDuration(get("NEBULA_ATTACHMENTS_TRANSFER_TIMEOUT", "5m"), "NEBULA_ATTACHMENTS_TRANSFER_TIMEOUT", &errs)
	a.PurgeDelay = parseNonNegDuration(get("NEBULA_ATTACHMENTS_PURGE_DELAY", "168h"), "NEBULA_ATTACHMENTS_PURGE_DELAY", &errs)
	if a.WorkspaceQuotaBytes > 0 && a.WorkspaceQuotaBytes < a.MaxFileBytes {
		errs = append(errs, errors.New("NEBULA_ATTACHMENTS_WORKSPACE_QUOTA_MB must be 0 or at least NEBULA_ATTACHMENTS_MAX_FILE_MB"))
	}

	switch a.Backend {
	case "disabled":
	case "local":
		if a.Dir == "" {
			errs = append(errs, errors.New("NEBULA_ATTACHMENTS_DIR is required for the local backend"))
		}
	case "s3":
		if a.S3.Endpoint == "" || a.S3.Bucket == "" || a.S3.AccessKey == "" || a.S3.SecretKey == "" {
			errs = append(errs, errors.New("the s3 attachment backend needs NEBULA_ATTACHMENTS_S3_ENDPOINT, _BUCKET, _ACCESS_KEY and _SECRET_KEY"))
		} else if u, err := url.Parse(a.S3.Endpoint); err != nil || (env == "production" && u.Scheme != "https") {
			errs = append(errs, errors.New("NEBULA_ATTACHMENTS_S3_ENDPOINT must be an https URL in production"))
		}
	default:
		errs = append(errs, fmt.Errorf("NEBULA_ATTACHMENTS_BACKEND must be local, s3 or disabled, got %q", a.Backend))
	}
	return a, errors.Join(errs...)
}

func parseNonNegDuration(raw, key string, errs *[]error) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 || d > 90*24*time.Hour {
		*errs = append(*errs, fmt.Errorf("%s must be a duration between 0s and 2160h", key))
	}
	return d
}
