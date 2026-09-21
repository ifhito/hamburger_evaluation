package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeHandoffRepo は、保存されたコードの中身を覚えておく、テスト用の repository である。
type fakeHandoffRepo struct {
	saved map[string]CreateLoginHandoffParams
}

func (f *fakeHandoffRepo) CreateLoginHandoff(_ context.Context, p CreateLoginHandoffParams) error {
	if f.saved == nil {
		f.saved = map[string]CreateLoginHandoffParams{}
	}
	f.saved[p.CodeHash] = p
	return nil
}

func (f *fakeHandoffRepo) DiscardLoginHandoff(_ context.Context, hash string) (LoginHandoff, error) {
	p, ok := f.saved[hash]
	if !ok {
		return LoginHandoff{}, ErrLoginHandoffInvalid
	}
	delete(f.saved, hash)
	return LoginHandoff{Outcome: p.Outcome, UserID: p.UserID, ReturnTo: p.ReturnTo}, nil
}

func (f *fakeHandoffRepo) DiscardExpiredLoginHandoffs(context.Context, int) (int64, error) {
	return 0, nil
}

func TestLoginHandoffsIssueAndRedeem(t *testing.T) {
	ctx := context.Background()

	t.Run("コードを発行すると、平文は保存されず、ハッシュだけが保存され、その平文で 1 回だけ中身を取り出せる", func(t *testing.T) {
		repo := &fakeHandoffRepo{}
		h := NewLoginHandoffs(repo)
		raw, err := h.Issue(ctx, OutcomeSignedIn, "user-1", "/shops")
		if err != nil {
			t.Fatal(err)
		}
		for hash := range repo.saved {
			if hash == raw || strings.Contains(hash, raw) {
				t.Fatal("平文のコードが保存されている")
			}
		}
		got, err := h.Redeem(ctx, raw)
		if err != nil || got.Outcome != OutcomeSignedIn || got.UserID != "user-1" || got.ReturnTo != "/shops" {
			t.Fatalf("got %+v, err %v", got, err)
		}
		if _, err := h.Redeem(ctx, raw); !errors.Is(err, ErrLoginHandoffInvalid) {
			t.Fatalf("2 回目は使えないはずが、err = %v", err)
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
		if _, err := h.Issue(ctx, OutcomeFailed, "user-1", ""); err == nil {
			t.Fatal("利用者つきの失敗を、発行できてしまった")
		}
		if len(repo.saved) != 0 {
			t.Fatal("拒否したのに、保存された")
		}
	})

	t.Run("知らないコードは、無効として断る", func(t *testing.T) {
		h := NewLoginHandoffs(&fakeHandoffRepo{})
		if _, err := h.Redeem(ctx, "unknown"); !errors.Is(err, ErrLoginHandoffInvalid) {
			t.Fatalf("err = %v", err)
		}
	})
}
