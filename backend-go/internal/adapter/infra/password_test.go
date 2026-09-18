package infra

import (
	"strings"
	"testing"
)

func TestBcryptPasswordHasher(t *testing.T) {
	hasher := BcryptPasswordHasher{}
	digest, err := hasher.Hash("password123")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}
	if digest == "password123" || !strings.HasPrefix(digest, "$2") {
		t.Fatalf("digest %q does not look like a bcrypt hash", digest)
	}
	if err := hasher.Compare(digest, "password123"); err != nil {
		t.Fatalf("Compare with correct password returned error: %v", err)
	}
	if err := hasher.Compare(digest, "wrong-password"); err == nil {
		t.Fatal("Compare with wrong password returned nil error, want error")
	}
}
