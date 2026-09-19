package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// s3API is the slice of the S3 client used here, split out so unit tests
// can fake it without a network.
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3 stores photos in an S3-compatible bucket and serves them from a
// public base URL (e.g. an R2 public bucket domain or CDN).
type S3 struct {
	client  s3API
	bucket  string
	baseURL string
}

var _ usecase.PhotoStorage = (*S3)(nil)

// NewS3 builds a client for an S3-compatible endpoint (e.g. the Cloudflare
// R2 S3 API) with static credentials. Path-style addressing is used
// because custom endpoints generally do not resolve bucket subdomains.
// No connection is made until the first call.
func NewS3(endpoint, bucket, accessKeyID, secretAccessKey, baseURL string) *S3 {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto", // R2's placeholder region
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		UsePathStyle: true,
	})
	return &S3{client: client, bucket: bucket, baseURL: strings.TrimSuffix(baseURL, "/")}
}

func (s *S3) Put(ctx context.Context, key string, contentType string, r io.Reader) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put photo %q: %w", key, err)
	}
	return nil
}

// Delete relies on S3 semantics: DeleteObject succeeds for missing keys,
// which gives the idempotency the contract requires.
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete photo %q: %w", key, err)
	}
	return nil
}

func (s *S3) URL(key string) string { return s.baseURL + "/" + key }
