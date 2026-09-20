package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// fakeS3 records the inputs; no network involved.
type fakeS3 struct {
	put    *s3.PutObjectInput
	delete *s3.DeleteObjectInput
}

func (f *fakeS3) PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.put = in
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.delete = in
	return &s3.DeleteObjectOutput{}, nil
}

// TestS3Put asserts bucket, key, content type, and body are forwarded to
// PutObject.
func TestS3Put(t *testing.T) {
	fake := &fakeS3{}
	s := &S3{client: fake, bucket: "photos", baseURL: "https://photos.example.com"}
	if err := s.Put(context.Background(), "reviews/1/a.jpg", "image/jpeg", strings.NewReader("jpeg-bytes")); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if fake.put == nil {
		t.Fatal("PutObject was not called")
	}
	if got := *fake.put.Bucket; got != "photos" {
		t.Fatalf("Bucket = %q, want %q", got, "photos")
	}
	if got := *fake.put.Key; got != "reviews/1/a.jpg" {
		t.Fatalf("Key = %q, want %q", got, "reviews/1/a.jpg")
	}
	if got := *fake.put.ContentType; got != "image/jpeg" {
		t.Fatalf("ContentType = %q, want %q", got, "image/jpeg")
	}
	body, err := io.ReadAll(fake.put.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(body) != "jpeg-bytes" {
		t.Fatalf("Body = %q, want %q", body, "jpeg-bytes")
	}
}

func TestS3Delete(t *testing.T) {
	fake := &fakeS3{}
	s := &S3{client: fake, bucket: "photos", baseURL: "https://photos.example.com"}
	if err := s.Delete(context.Background(), "reviews/1/a.jpg"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if fake.delete == nil {
		t.Fatal("DeleteObject was not called")
	}
	if got := *fake.delete.Bucket; got != "photos" {
		t.Fatalf("Bucket = %q, want %q", got, "photos")
	}
	if got := *fake.delete.Key; got != "reviews/1/a.jpg" {
		t.Fatalf("Key = %q, want %q", got, "reviews/1/a.jpg")
	}
}

// TestS3URL covers NewS3's trailing-slash trim and URL construction.
func TestS3URL(t *testing.T) {
	s := NewS3("https://acct.r2.cloudflarestorage.com", "photos", "id", "secret", "https://photos.example.com/")
	if got := s.URL("reviews/1/a.jpg"); got != "https://photos.example.com/reviews/1/a.jpg" {
		t.Fatalf("URL = %q, want %q", got, "https://photos.example.com/reviews/1/a.jpg")
	}
}

// TestS3KeyValidation asserts the shared traversal guard applies before
// any client call.
func TestS3KeyValidation(t *testing.T) {
	fake := &fakeS3{}
	s := &S3{client: fake, bucket: "photos", baseURL: "https://photos.example.com"}
	for _, key := range []string{"", "/abs.jpg", "../escape.jpg", `a\b.jpg`} {
		if err := s.Put(context.Background(), key, "image/jpeg", strings.NewReader("x")); err == nil {
			t.Errorf("Put(%q) returned nil error, want rejection", key)
		}
		if err := s.Delete(context.Background(), key); err == nil {
			t.Errorf("Delete(%q) returned nil error, want rejection", key)
		}
	}
	if fake.put != nil || fake.delete != nil {
		t.Fatal("client was called for an invalid key")
	}
}
