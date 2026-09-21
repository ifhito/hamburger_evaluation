package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeHandoffRepo は、保存されたコードの中身を覚えておく、テスト用の repository である。
type fakeHandoffRepo struct {
	saved  map[string]CreateLoginHandoffParams
	locked []string
}

func (f *fakeHandoffRepo) CreateLoginHandoff(_ context.Context, p CreateLoginHandoffParams) error {
	if f.saved == nil {
		f.saved = map[string]CreateLoginHandoffParams{}
	}
	f.saved[p.CodeHash] = p
	return nil
}

func (f *fakeHandoffRepo) LockLoginHandoff(_ context.Context, hash string) error {
	if _, ok := f.saved[hash]; !ok {
		return ErrLoginHandoffInvalid
	}
	f.locked = append(f.locked, hash)
	return nil
}

func (f *fakeHandoffRepo) DiscardLoginHandoff(context.Context, string) error { return nil }

func (f *fakeHandoffRepo) DiscardExpiredLoginHandoffs(context.Context, int) (int64, error) {
	return 0, nil
}

func TestLoginHandoffsIssueAndLock(t *testing.T) {
	ctx := context.Background()

	t.Run("コードを発行すると、平文は保存されず、ハッシュだけが保存され、その平文でロックできる", func(t *testing.T) {
		repo := &fakeHandoffRepo{}
		h := NewLoginHandoffs(repo)
		raw, err := h.Issue(ctx, OutcomeSignedIn, "user-1", "/shops")
		if err != nil {
			t.Fatal(err)
		}
		for hash, p := range repo.saved {
			if hash == raw || strings.Contains(hash, raw) || p.CodeHash != hash {
				t.Fatal("平文のコードが保存されている")
			}
			if p.CodeHash != HashLoginHandoffCode(raw) || p.Outcome != OutcomeSignedIn || p.UserID != "user-1" || p.ReturnTo != "/shops" {
				t.Fatalf("保存された内容 = %+v", p)
			}
		}
		if err := h.Lock(ctx, raw); err != nil {
			t.Fatalf("Lock: %v", err)
		}
		if len(repo.locked) != 1 || repo.locked[0] != HashLoginHandoffCode(raw) {
			t.Fatalf("ロックされたもの = %v, want 平文のハッシュ", repo.locked)
		}
	})

	t.Run("発行のたびに、違うコードが作られる", func(t *testing.T) {
		h := NewLoginHandoffs(&fakeHandoffRepo{})
		a, _ := h.Issue(ctx, OutcomeFailed, "", "")
		b, _ := h.Issue(ctx, OutcomeFailed, "", "")
		if a == b || len(a) < 43 {
			t.Fatalf("コードが短いか、重複している: %q %q", a, b)
		}
	})

	t.Run("結果の種類と利用者の ID が食い違う発行は、保存せずに拒否する", func(t *testing.T) {
		repo := &fakeHandoffRepo{}
		h := NewLoginHandoffs(repo)
		if _, err := h.Issue(ctx, OutcomeSignedIn, "", ""); err == nil {
			t.Fatal("利用者なしのサインインの成功を、発行できてしまった")
		}
		if _, err := h.Issue(ctx, OutcomeLinked, "", ""); err == nil {
			t.Fatal("利用者なしの結び付けの成功を、発行できてしまった")
		}
		if _, err := h.Issue(ctx, OutcomeFailed, "user-1", ""); err == nil {
			t.Fatal("利用者つきの失敗を、発行できてしまった")
		}
		if len(repo.saved) != 0 {
			t.Fatal("拒否したのに、保存された")
		}
	})

	t.Run("知らないコードは、ロックできず、無効として断る", func(t *testing.T) {
		h := NewLoginHandoffs(&fakeHandoffRepo{})
		if err := h.Lock(ctx, "unknown"); !errors.Is(err, ErrLoginHandoffInvalid) {
			t.Fatalf("err = %v", err)
		}
	})
}
