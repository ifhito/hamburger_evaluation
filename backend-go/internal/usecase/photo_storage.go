package usecase

import (
	"context"
	"io"
)

// PhotoStorage is the consumer-side blob-storage contract for review
// photos (implemented in adapter/storage: local disk and S3-compatible).
// Keys are server-generated, slash-separated relative paths including the
// file extension (e.g. "reviews/42/a1b2c3.jpg").
type PhotoStorage interface {
	// Put stores the bytes read from r under key with the given content
	// type, replacing any existing object.
	Put(ctx context.Context, key string, contentType string, r io.Reader) error
	// Delete removes the object under key. Deleting a missing key is NOT
	// an error (idempotent).
	Delete(ctx context.Context, key string) error
	// URL returns the public URL clients use to fetch the photo.
	URL(key string) string
}
