// Package storage は usecase.PhotoStorage の契約を実装する：開発用の
// ローカルディスクのストアと、本番用の S3 互換ストア（Cloudflare R2 の
// S3 API）である。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// validateKey は、空のキー、絶対パスのキー、バックスラッシュを含むキー、
// ".." を含むキーを拒否する。キーはサーバが生成するものなので、これは
// 入力検証ではなく、path traversal に対する多層防御である。
func validateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "..") || strings.Contains(key, `\`) {
		return fmt.Errorf("invalid photo key %q", key)
	}
	return nil
}

// Disk は、ルートディレクトリの下にファイルとして写真を保存し、公開 base URL
// （例："/photos"）から配信する。
type Disk struct {
	root    string
	baseURL string
}

var _ usecase.PhotoStorage = (*Disk)(nil)

func NewDisk(root, baseURL string) *Disk {
	return &Disk{root: root, baseURL: strings.TrimSuffix(baseURL, "/")}
}

// Put はアトミックに書き込む。バイト列は対象ディレクトリ内の一時ファイルに
// 書かれ、それが最終的なパスへ rename で上書きされるので、読み手が中途半端な
// 写真を目にすることはない。content type は保存されず、disk からの配信では
// ファイル拡張子からそれを導出する。
func (d *Disk) Put(ctx context.Context, key string, contentType string, r io.Reader) error {
	if err := validateKey(key); err != nil {
		return err
	}
	path := filepath.Join(d.root, filepath.FromSlash(key))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("put photo %q: %w", key, err)
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return fmt.Errorf("put photo %q: %w", key, err)
	}
	defer os.Remove(tmp.Name()) // rename が成功した後は no-op になる
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return fmt.Errorf("put photo %q: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("put photo %q: %w", key, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("put photo %q: %w", key, err)
	}
	return nil
}

// Delete はファイルを削除する。存在しないキーはエラーではない（冪等）。
func (d *Disk) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(d.root, filepath.FromSlash(key))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete photo %q: %w", key, err)
	}
	return nil
}

func (d *Disk) URL(key string) string { return d.baseURL + "/" + key }
