package main

import (
	"log/slog"

	"github.com/Tural-HS/Nebula/internal/blobstore"
	"github.com/Tural-HS/Nebula/internal/config"
	"github.com/Tural-HS/Nebula/internal/server"
)

// newBlobStore opens the attachment backend (ADR-0022); nil when disabled.
// The local backend checks at startup that its volume is writable.
func newBlobStore(c config.Attachments, logger *slog.Logger) (blobstore.Store, error) {
	switch c.Backend {
	case "disabled":
		logger.Warn("NEBULA_ATTACHMENTS_BACKEND=disabled: file attachments are off")
		return nil, nil
	case "s3":
		s, err := blobstore.NewS3(blobstore.S3Config{
			Endpoint: c.S3.Endpoint, Region: c.S3.Region, Bucket: c.S3.Bucket,
			AccessKey: c.S3.AccessKey, SecretKey: c.S3.SecretKey, PathStyle: c.S3.PathStyle,
			Prefix: c.S3.Prefix, TempDir: c.TempDir,
		})
		if err != nil {
			return nil, err
		}
		return s, nil
	default:
		l, err := blobstore.NewLocal(c.Dir)
		if err != nil {
			return nil, err
		}
		return l, nil
	}
}

func attachmentDeps(c config.Attachments, blobs blobstore.Store) server.Attachments {
	return server.Attachments{
		Blobs: blobs, MaxFileBytes: c.MaxFileBytes, WorkspaceQuotaBytes: c.WorkspaceQuotaBytes,
		MaxPerItem: c.MaxPerItem, UploadsPerMinute: c.UploadsPerMinute, TransferTimeout: c.TransferTimeout,
	}
}
