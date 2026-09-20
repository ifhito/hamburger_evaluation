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

// s3API は、ここで使う S3 client の一部分であり、単体テストがネットワークなしで
// fake できるように切り出してある。
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3 は、S3 互換のバケットに写真を保存し、公開 base URL（例：R2 の公開バケット
// のドメインや CDN）から配信する。
type S3 struct {
	client  s3API
	bucket  string
	baseURL string
}

var _ usecase.PhotoStorage = (*S3)(nil)

// NewS3 は、S3 互換のエンドポイント（例：Cloudflare R2 の S3 API）向けの
// client を、静的な credentials で組み立てる。カスタムエンドポイントは一般に
// バケットのサブドメインを解決しないので、path-style のアドレッシングを使う。
// 最初の呼び出しまで接続は行われない。
func NewS3(endpoint, bucket, accessKeyID, secretAccessKey, baseURL string) *S3 {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto", // R2 のプレースホルダのリージョン
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

// Delete は S3 のセマンティクスに依拠する。DeleteObject は存在しないキーに
// 対しても成功し、これが契約の求める冪等性をもたらす。
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
