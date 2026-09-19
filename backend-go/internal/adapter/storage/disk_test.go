package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
)

// TestDiskPutReadBack asserts Put writes the exact bytes under root/key,
// creating parent directories, and leaves no temp files behind.
func TestDiskPutReadBack(t *testing.T) {
	root := t.TempDir()
	d := storage.NewDisk(root, "/photos")
	key := "reviews/42/abc.jpg"
	if err := d.Put(context.Background(), key, "image/jpeg", strings.NewReader("jpeg-bytes")); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "reviews", "42", "abc.jpg"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "jpeg-bytes" {
		t.Fatalf("stored content = %q, want %q", got, "jpeg-bytes")
	}
	entries, err := os.ReadDir(filepath.Join(root, "reviews", "42"))
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "abc.jpg" {
		t.Fatalf("directory not clean after Put: %v", entries)
	}
}

// TestDiskPutOverwrites asserts a second Put atomically replaces the
// stored bytes.
func TestDiskPutOverwrites(t *testing.T) {
	root := t.TempDir()
	d := storage.NewDisk(root, "/photos")
	for _, content := range []string{"first", "second"} {
		if err := d.Put(context.Background(), "a.png", "image/png", strings.NewReader(content)); err != nil {
			t.Fatalf("Put(%q) returned error: %v", content, err)
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "a.png"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("stored content = %q, want %q", got, "second")
	}
}

// TestDiskDelete asserts Delete removes the file and that deleting a
// missing key is nil (idempotent).
func TestDiskDelete(t *testing.T) {
	root := t.TempDir()
	d := storage.NewDisk(root, "/photos")
	if err := d.Put(context.Background(), "a.jpg", "image/jpeg", strings.NewReader("x")); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if err := d.Delete(context.Background(), "a.jpg"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.jpg")); !os.IsNotExist(err) {
		t.Fatalf("file still exists after Delete (stat err: %v)", err)
	}
	if err := d.Delete(context.Background(), "a.jpg"); err != nil {
		t.Fatalf("Delete of missing key returned error: %v, want nil", err)
	}
}

func TestDiskURL(t *testing.T) {
	d := storage.NewDisk(t.TempDir(), "/photos")
	if got := d.URL("reviews/1/a.jpg"); got != "/photos/reviews/1/a.jpg" {
		t.Fatalf("URL = %q, want %q", got, "/photos/reviews/1/a.jpg")
	}
	trailing := storage.NewDisk(t.TempDir(), "https://cdn.example.com/photos/")
	if got := trailing.URL("a.jpg"); got != "https://cdn.example.com/photos/a.jpg" {
		t.Fatalf("URL = %q, want %q", got, "https://cdn.example.com/photos/a.jpg")
	}
}

// TestDiskKeyValidation asserts traversal-shaped keys are rejected by Put
// and Delete alike, and that nothing escapes the root.
func TestDiskKeyValidation(t *testing.T) {
	root := t.TempDir()
	d := storage.NewDisk(root, "/photos")
	for _, key := range []string{"", "/abs.jpg", "../escape.jpg", "a/../b.jpg", `a\b.jpg`} {
		if err := d.Put(context.Background(), key, "image/jpeg", strings.NewReader("x")); err == nil {
			t.Errorf("Put(%q) returned nil error, want rejection", key)
		}
		if err := d.Delete(context.Background(), key); err == nil {
			t.Errorf("Delete(%q) returned nil error, want rejection", key)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.jpg")); !os.IsNotExist(err) {
		t.Fatalf("traversal key escaped the root (stat err: %v)", err)
	}
}
