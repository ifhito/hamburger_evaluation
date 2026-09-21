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
		issued, err := h.Issue(ctx, OutcomeSignedIn, "user-1", "/shops")
		if err != nil {
			t.Fatal(err)
		}
		raw := issued.Code
		for hash, p := range repo.saved {
			for _, secret := range []string{issued.Code, issued.Binder} {
				if hash == secret || strings.Contains(hash, secret) || strings.Contains(p.BinderHash, secret) {
					t.Fatal("平文のコードか結び付けの値が保存されている")
				}
			}
			if p.CodeHash != hash {
				t.Fatalf("保存の鍵とコードのハッシュが違う: %q %q", hash, p.CodeHash)
			}
			if p.CodeHash != HashLoginHandoffCode(raw) || p.BinderHash != HashLoginHandoffBinder(issued.Binder) ||
				p.Outcome != OutcomeSignedIn || p.UserID != "user-1" || p.ReturnTo != "/shops" {
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
		if a.Code == b.Code || len(a.Code) < 43 {
			t.Fatalf("コードが短いか、重複している: %q %q", a.Code, b.Code)
		}
		if a.Binder == b.Binder || len(a.Binder) < 43 || a.Binder == a.Code {
			t.Fatalf("結び付けの値が短いか、重複しているか、コードと同じ: %q %q", a.Binder, b.Binder)
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

func TestLoginHandoffBoundTo(t *testing.T) {
	ctx := context.Background()
	repo := &fakeHandoffRepo{}
	issued, err := NewLoginHandoffs(repo).Issue(ctx, OutcomeFailed, "", "")
	if err != nil {
		t.Fatal(err)
	}
	h := LoginHandoff{BinderHash: repo.saved[HashLoginHandoffCode(issued.Code)].BinderHash}

	t.Run("発行のときの結び付けの値だけが合う", func(t *testing.T) {
		if !h.BoundTo(issued.Binder) {
			t.Fatal("発行のときの結び付けの値が合わない")
		}
	})

	t.Run("空・別の値・コードそのものは合わない(コードだけでは、使えない)", func(t *testing.T) {
		for name, binder := range map[string]string{"空": "", "別の値": "another-binder", "コードそのもの": issued.Code} {
			if h.BoundTo(binder) {
				t.Errorf("%s が合ってしまった", name)
			}
		}
	})

	t.Run("保存された結び付けの値が空なら、空の値でも合わない", func(t *testing.T) {
		if (LoginHandoff{}).BoundTo("") || (LoginHandoff{}).BoundTo("x") {
			t.Fatal("結び付けの値のないコードが、合ってしまった")
		}
	})
}
