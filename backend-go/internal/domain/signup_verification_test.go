package domain

import (
	"context"
	"encoding/base64"
	"regexp"
	"testing"
)

func TestNewSignupToken(t *testing.T) {
	hexSHA256 := regexp.MustCompile(`^[0-9a-f]{64}$`)

	t.Run("平文は 32 バイトの乱数を base64url にしたもので、Hash はその SHA-256 になる", func(t *testing.T) {
		token, err := NewSignupToken()
		if err != nil {
			t.Fatalf("NewSignupToken returned error: %v", err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(token.Raw)
		if err != nil {
			t.Fatalf("Raw %q は base64url ではない: %v", token.Raw, err)
		}
		if len(decoded) != 32 {
			t.Errorf("Raw の乱数は %d バイト、want 32", len(decoded))
		}
		if !hexSHA256.MatchString(token.Hash) {
			t.Errorf("Hash %q は SHA-256 の 16 進(64 文字)ではない", token.Hash)
		}
		if token.Hash != HashSignupToken(token.Raw) {
			t.Errorf("Hash = %q、HashSignupToken(Raw) = %q で食い違う", token.Hash, HashSignupToken(token.Raw))
		}
		if token.Raw == token.Hash {
			t.Error("Raw と Hash が同じ値になっている(平文をそのまま保存する形になる)")
		}
	})

	t.Run("呼ぶたびに別のトークンになる", func(t *testing.T) {
		seen := map[string]bool{}
		for i := 0; i < 100; i++ {
			token, err := NewSignupToken()
			if err != nil {
				t.Fatalf("NewSignupToken returned error: %v", err)
			}
			if seen[token.Raw] {
				t.Fatalf("同じ Raw が重複した: %q", token.Raw)
			}
			seen[token.Raw] = true
		}
	})
}

func TestHashSignupToken(t *testing.T) {
	// SHA-256("abc") の既知の値。
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashSignupToken("abc"); got != want {
		t.Errorf("HashSignupToken(abc) = %q, want %q", got, want)
	}
	if HashSignupToken("abc") != HashSignupToken("abc") {
		t.Error("同じ入力で結果が変わった")
	}
	if HashSignupToken("abc") == HashSignupToken("abd") {
		t.Error("違う入力が同じハッシュになった")
	}
}

// recordingSignupRepo は、SignupVerifications が repository へ渡した値を記録する fake である。
type recordingSignupRepo struct {
	created      []CreateSignupVerificationParams
	locked       []string
	discardedIDs []string
	discarded    []int
}

func (r *recordingSignupRepo) CreateSignupVerification(_ context.Context, p CreateSignupVerificationParams) (SignupVerificationReceipt, error) {
	r.created = append(r.created, p)
	return SignupVerificationReceipt{Accepted: true, ID: "id", Generation: 1}, nil
}

func (r *recordingSignupRepo) LockSignupVerification(_ context.Context, tokenHash string) error {
	r.locked = append(r.locked, tokenHash)
	return nil
}

func (r *recordingSignupRepo) DiscardSignupVerification(_ context.Context, id string) error {
	r.discardedIDs = append(r.discardedIDs, id)
	return nil
}

func (r *recordingSignupRepo) DiscardExpiredSignupVerifications(_ context.Context, limit int) (int64, error) {
	r.discarded = append(r.discarded, limit)
	return 0, nil
}

func TestSignupVerificationsLockPassesOnlyTheHash(t *testing.T) {
	repo := &recordingSignupRepo{}
	if err := NewSignupVerifications(repo).Lock(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Lock returned error: %v", err)
	}
	if len(repo.locked) != 1 || repo.locked[0] != HashSignupToken("raw-token") {
		t.Errorf("repository へ渡した値 = %v、want [HashSignupToken(raw-token)]（平文を渡してはならない）", repo.locked)
	}
}

func TestSignupVerificationsDiscardPassesTheID(t *testing.T) {
	repo := &recordingSignupRepo{}
	if err := NewSignupVerifications(repo).Discard(context.Background(), "verification-id"); err != nil {
		t.Fatalf("Discard returned error: %v", err)
	}
	if len(repo.discardedIDs) != 1 || repo.discardedIDs[0] != "verification-id" {
		t.Errorf("repository へ渡した id = %v、want [verification-id]", repo.discardedIDs)
	}
}
