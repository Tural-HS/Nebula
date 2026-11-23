package blobstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

// S3Config configures an S3-compatible bucket (AWS S3, MinIO, Garage, Ceph…).
type S3Config struct {
	Endpoint  string // https://s3.example.test (scheme + host[:port], no path)
	Region    string // e.g. us-east-1 (MinIO accepts any)
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle addresses objects as <endpoint>/<bucket>/<key> (MinIO and
	// most self-hosted servers); false uses <bucket>.<endpoint host>/<key>.
	PathStyle bool
	// Prefix is prepended to every object key, e.g. "nebula/attachments/".
	Prefix string
	// TempDir holds staging files (the container's tmpfs). Empty: os.TempDir().
	TempDir string
	// HTTP is the client; nil uses one with sane timeouts.
	HTTP *http.Client
}

// S3 stores blobs in a bucket through the plain REST API, signed with AWS
// Signature Version 4. Only PUT, GET and DELETE of single objects are needed,
// so a small signer replaces a large SDK (ADR-0022). The payload hash is the
// real SHA-256 of the upload, so the server rejects any corrupted transfer.
type S3 struct {
	cfg  S3Config
	base *url.URL
	http *http.Client
	now  func() time.Time
}

// NewS3 validates the configuration. It does not contact the server.
func NewS3(cfg S3Config) (*S3, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.User != nil {
		return nil, errors.New("S3 endpoint must be an absolute http(s) URL without path, query or credentials")
	}
	u.Path = ""
	if cfg.Region == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("S3 region, bucket, access key and secret key are required")
	}
	if strings.ContainsAny(cfg.Bucket, "/?#") || strings.Contains(cfg.Prefix, "..") {
		return nil, errors.New("invalid S3 bucket or prefix")
	}
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	c := cfg.HTTP
	if c == nil {
		c = &http.Client{
			Timeout: 10 * time.Minute, // whole transfer; per-request contexts are tighter
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse // never follow a redirect with a signed request
			},
		}
	}
	return &S3{cfg: cfg, base: u, http: c, now: time.Now}, nil
}

// Name implements Store.
func (s *S3) Name() string { return "s3" }

// CreateTemp implements Store.
func (s *S3) CreateTemp() (*os.File, error) {
	return os.CreateTemp(s.cfg.TempDir, "nebula-upload-*")
}

func (s *S3) objectURL(key string) *url.URL {
	u := *s.base
	obj := s.cfg.Prefix + key
	if s.cfg.PathStyle {
		u.Path = "/" + s.cfg.Bucket + "/" + obj
	} else {
		u.Host = s.cfg.Bucket + "." + u.Host
		u.Path = "/" + obj
	}
	return &u
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Commit implements Store.
func (s *S3) Commit(ctx context.Context, key string, f *os.File, size int64, sum []byte) error {
	defer Discard(f)
	if !ValidKey(key) || len(sum) != sha256.Size {
		return errBadKey
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objectURL(key).String(), io.NopCloser(f))
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := s.do(req, hex.EncodeToString(sum))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	return drain(res, http.StatusOK)
}

// Open implements Store.
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !ValidKey(key) {
		return nil, errBadKey
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objectURL(key).String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := s.do(req, emptySHA256)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusOK {
		return res.Body, nil
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		_ = drain(res, http.StatusNotFound)
		return nil, ErrNotFound
	}
	return nil, drain(res, http.StatusOK)
}

// Delete implements Store.
func (s *S3) Delete(ctx context.Context, key string) error {
	if !ValidKey(key) {
		return errBadKey
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objectURL(key).String(), nil)
	if err != nil {
		return err
	}
	res, err := s.do(req, emptySHA256)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		return drain(res, http.StatusNotFound)
	}
	return drain(res, http.StatusNoContent, http.StatusOK)
}

func (s *S3) do(req *http.Request, payloadHash string) (*http.Response, error) {
	signV4(req, payloadHash, s.cfg.Region, s.cfg.AccessKey, s.cfg.SecretKey, s.now().UTC())
	res, err := s.http.Do(req)
	if err != nil {
		// Never include the URL's query or headers; the error names the host only.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("s3 %s: %w", req.Method, ue.Err)
		}
		return nil, fmt.Errorf("s3 %s: %w", req.Method, err)
	}
	return res, nil
}

// drain consumes the body (the caller closes it) and turns unexpected
// statuses into errors that carry the S3 error code (not the body, which may
// echo request details).
func drain(res *http.Response, ok ...int) error {
	if slices.Contains(ok, res.StatusCode) {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	code := "unknown"
	if _, rest, found := strings.Cut(string(b), "<Code>"); found {
		if c, _, found := strings.Cut(rest, "</Code>"); found && len(c) < 64 {
			code = c
		}
	}
	return fmt.Errorf("s3: unexpected status %d (%s)", res.StatusCode, code)
}

// signV4 adds AWS Signature Version 4 headers (Authorization, X-Amz-Date,
// X-Amz-Content-Sha256) to req. Every header already set on the request,
// plus Host, is signed.
func signV4(req *http.Request, payloadHash, region, accessKey, secretKey string, now time.Time) {
	const service = "s3"
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = strings.Join(trimAll(v), ",")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	slices.Sort(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method,
		canonicalPath(req.URL.Path),
		canonicalQuery(req.URL.Query()),
		canonHeaders.String(),
		signed,
		payloadHash,
	}, "\n")
	scope := day + "/" + region + "/" + service + "/aws4_request"
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(h[:])

	k := hmacSHA256([]byte("AWS4"+secretKey), day)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
}

func trimAll(vs []string) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = strings.Join(strings.Fields(v), " ")
	}
	return out
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// canonicalPath URI-encodes each segment once (S3 does not double-encode).
func canonicalPath(p string) string {
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = uriEncode(s)
	}
	return strings.Join(segs, "/")
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var parts []string
	for _, k := range keys {
		vs := slices.Clone(q[k])
		slices.Sort(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k)+"="+uriEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

// uriEncode is RFC 3986 encoding with AWS's unreserved set.
func uriEncode(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
