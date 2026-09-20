// Package storage implements the usecase.PhotoStorage contract: a
// local-disk store for development and an S3-compatible store (Cloudflare
// R2's S3 API) for production.
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

// validateKey rejects empty, absolute, backslashed, or ".."-containing
// keys. Keys are server-generated, so this is defense in depth against
// path traversal, not input validation.
func validateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "..") || strings.Contains(key, `\`) {
		return fmt.Errorf("invalid photo key %q", key)
	}
	return nil
}

// Disk stores photos as files under a root directory and serves them from
// a public base URL (e.g. "/photos").
type Disk struct {
	root    string
	baseURL string
}

var _ usecase.PhotoStorage = (*Disk)(nil)

func NewDisk(root, baseURL string) *Disk {
	return &Disk{root: root, baseURL: strings.TrimSuffix(baseURL, "/")}
}

// Put writes atomically: the bytes go to a temp file in the target
// directory, which is then renamed over the final path, so readers never
// see a partial photo. The content type is not stored; disk serving
// derives it from the file extension.
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
	defer os.Remove(tmp.Name()) // no-op once the rename succeeded
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

// Delete removes the file; a missing key is not an error (idempotent).
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
