package usecase

import (
	"context"
	"io"
)

// PhotoStorage は review の写真向けの consumer 側の blob storage の契約である
// （adapter/storage で実装される：ローカルディスクと S3 互換）。key は
// サーバーが生成する、スラッシュ区切りの相対パスで、ファイル拡張子を含む
// （例："reviews/42/a1b2c3.jpg"）。
type PhotoStorage interface {
	// Put は、r から読み取ったバイト列を、指定された content type で key の
	// 下に保存し、既存のオブジェクトがあれば置き換える。
	Put(ctx context.Context, key string, contentType string, r io.Reader) error
	// Delete は key の下のオブジェクトを削除する。存在しない key の削除は
	// エラーでは「ない」（冪等）。
	Delete(ctx context.Context, key string) error
	// URL は、クライアントが写真を取得するのに使う公開 URL を返す。
	URL(key string) string
}
