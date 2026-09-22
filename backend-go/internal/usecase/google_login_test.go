package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeGoogleProvider は、Google が確かめた利用者の情報を、決まった値で返す usecase.GoogleProvider の代役である。
type fakeGoogleProvider struct{ identity domain.ExternalIdentity }

func (fakeGoogleProvider) Begin(context.Context) (string, usecase.GoogleFlowSecrets, error) {
	panic("unexpected Begin call")
}

func (f fakeGoogleProvider) Complete(context.Context, string, usecase.GoogleFlowSecrets) (domain.ExternalIdentity, error) {
	return f.identity, nil
}

// fakeIdentityQuery は、結び付きの読み取り(usecase.IdentityQuery)の代役である。lookups は、呼ばれるたびに、
// 前から順に使う結果である(足りなくなったら、最後のものを使い続ける)。
type fakeIdentityQuery struct {
	lookups []func() (domain.UserIdentity, error)
	calls   int
}

func (f *fakeIdentityQuery) GetIdentityByProviderUserID(context.Context, string, string) (domain.UserIdentity, error) {
	i := f.calls
	if i >= len(f.lookups) {
		i = len(f.lookups) - 1
	}
	f.calls++
	return f.lookups[i]()
}

func (f *fakeIdentityQuery) ListIdentitiesByUser(context.Context, string) ([]domain.UserIdentity, error) {
	panic("unexpected ListIdentitiesByUser call")
}

func notLinked() (domain.UserIdentity, error) {
	return domain.UserIdentity{}, fmt.Errorf("lookup: %w", domain.ErrIdentityNotFound)
}

func linkedTo(userID string) func() (domain.UserIdentity, error) {
	return func() (domain.UserIdentity, error) {
		return domain.UserIdentity{UserID: userID, Provider: domain.ProviderGoogle, ProviderUserID: "sub-1"}, nil
	}
}

// fakeIdentityRepo は、結び付きの書き込み(domain.UserIdentityRepository)の代役である。
type fakeIdentityRepo struct {
	create func(context.Context, domain.CreateUserIdentityParams) (domain.UserIdentity, error)
}

func (f fakeIdentityRepo) CreateUserIdentity(ctx context.Context, p domain.CreateUserIdentityParams) (domain.UserIdentity, error) {
	if f.create == nil {
		panic("unexpected CreateUserIdentity call")
	}
	return f.create(ctx, p)
}

func (fakeIdentityRepo) DiscardUserIdentity(context.Context, string, string) error {
	panic("unexpected DiscardUserIdentity call")
}

func (fakeIdentityRepo) DiscardUserIdentitiesByUser(context.Context, string) error {
	panic("unexpected DiscardUserIdentitiesByUser call")
}

// fakeHandoffRepo は、画面へ渡すコードの書き込み(domain.LoginHandoffRepository)の代役で、保存された中身と、
// 削除された ID を記録する。
type fakeHandoffRepo struct {
	created   []domain.CreateLoginHandoffParams
	locked    []string
	discarded []string
}

func (f *fakeHandoffRepo) CreateLoginHandoff(_ context.Context, p domain.CreateLoginHandoffParams) error {
	f.created = append(f.created, p)
	return nil
}

func (f *fakeHandoffRepo) LockLoginHandoff(_ context.Context, codeHash string) error {
	f.locked = append(f.locked, codeHash)
	return nil
}

func (f *fakeHandoffRepo) DiscardLoginHandoff(_ context.Context, id string) error {
	f.discarded = append(f.discarded, id)
	return nil
}

func (*fakeHandoffRepo) DiscardExpiredLoginHandoffs(context.Context, int) (int64, error) {
	return 0, nil
}

// TestGoogleLoginsSignInRace は、同じ Google アカウントの初回のサインインが並行して、ユーザーの作成が負けたときの
// 結果を固定する。負けた側は、先に作られた利用者としてサインインでき、「同じメールのアカウントがある」とは案内されない。
func TestGoogleLoginsSignInRace(t *testing.T) {
	winner := domain.User{ID: uid.N(7), Username: "Carol", Email: "carol@gmail.example"}
	ident := domain.ExternalIdentity{Provider: domain.ProviderGoogle, ProviderUserID: "sub-1", Email: "carol@gmail.example", EmailVerified: true, Name: "Carol"}

	// run は、サインインの手続きを進めて、画面へ渡すコードに入った結果を返す。
	run := func(t *testing.T, lookups []func() (domain.UserIdentity, error), userCreate error, identityCreate error, getByID func(string) (domain.User, error)) domain.CreateLoginHandoffParams {
		t.Helper()
		handoffs := &fakeHandoffRepo{}
		users := &fakeUserQuery{
			getByEmailIgnoreCase: func(context.Context, string) (domain.User, error) {
				return domain.User{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
			},
			getByID: func(_ context.Context, id string) (domain.User, error) { return getByID(id) },
		}
		userRepo := &fakeUserRepo{createUser: func(context.Context, domain.CreateUserParams) (domain.User, error) {
			if userCreate != nil {
				return domain.User{}, fmt.Errorf("create: %w", userCreate)
			}
			return domain.User{ID: uid.N(9), Username: "Carol", Email: ident.Email}, nil
		}}
		identityRepo := fakeIdentityRepo{create: func(context.Context, domain.CreateUserIdentityParams) (domain.UserIdentity, error) {
			return domain.UserIdentity{}, fmt.Errorf("create: %w", identityCreate)
		}}
		logins := usecase.NewGoogleLogins(fakeGoogleProvider{identity: ident}, &fakeIdentityQuery{lookups: lookups}, users,
			&uowtest.UoW{Users: userRepo, UserIdentities: identityRepo}, domain.NewLoginHandoffs(handoffs), domain.NewUserIdentities(identityRepo), fakeIssuer{})
		flow := usecase.GoogleFlow{Secrets: usecase.GoogleFlowSecrets{State: "s", Nonce: "n", Verifier: "v"}}
		if _, err := logins.Complete(context.Background(), flow, usecase.GoogleCallback{Code: "c", State: "s"}); err != nil {
			t.Fatal(err)
		}
		if len(handoffs.created) != 1 {
			t.Fatalf("保存された結果 = %d 件, want 1", len(handoffs.created))
		}
		return handoffs.created[0]
	}
	active := func(id string) (domain.User, error) {
		if id == winner.ID {
			return winner, nil
		}
		return domain.User{}, fmt.Errorf("get: %w", domain.ErrUserNotFound)
	}

	t.Run("ユーザーの作成がメールの重複で負けても、結び付きを引き直して、先に作られた利用者としてサインインする", func(t *testing.T) {
		got := run(t, []func() (domain.UserIdentity, error){notLinked, linkedTo(winner.ID)}, domain.ErrEmailTaken, nil, active)
		if got.Outcome != domain.OutcomeSignedIn || got.UserID != winner.ID {
			t.Fatalf("結果 = %+v, want %s の利用者としてのサインイン", got, winner.ID)
		}
	})

	t.Run("結び付きの作成が、すでに結び付いていると負けても、先に結び付いた利用者としてサインインする(作りかけの利用者ではない)", func(t *testing.T) {
		got := run(t, []func() (domain.UserIdentity, error){notLinked, linkedTo(winner.ID)}, nil, domain.ErrIdentityTaken, active)
		if got.Outcome != domain.OutcomeSignedIn || got.UserID != winner.ID {
			t.Fatalf("結果 = %+v, want %s の利用者としてのサインイン", got, winner.ID)
		}
	})

	t.Run("引き直しても結び付きがなく、メールの重複で負けたときは、別のアカウントがあるので、案内の結果になる", func(t *testing.T) {
		got := run(t, []func() (domain.UserIdentity, error){notLinked}, domain.ErrEmailTaken, nil, active)
		if got.Outcome != domain.OutcomeAccountExists || got.UserID != "" {
			t.Fatalf("結果 = %+v, want account_exists", got)
		}
	})

	t.Run("引き直しても結び付きがなく、結び付きの重複で負けたときは、失敗になる(サインインさせない)", func(t *testing.T) {
		got := run(t, []func() (domain.UserIdentity, error){notLinked}, nil, domain.ErrIdentityTaken, active)
		if got.Outcome != domain.OutcomeFailed || got.UserID != "" {
			t.Fatalf("結果 = %+v, want failed", got)
		}
	})

	t.Run("引き直した結び付きの持ち主が退会済みなら、サインインさせず、失敗になる", func(t *testing.T) {
		got := run(t, []func() (domain.UserIdentity, error){notLinked, linkedTo(uid.N(99))}, domain.ErrEmailTaken, nil, active)
		if got.Outcome != domain.OutcomeFailed || got.UserID != "" {
			t.Fatalf("結果 = %+v, want failed", got)
		}
	})

	t.Run("引き直しの読み取りが失敗したときは、失敗になる(案内にも、サインインにもしない)", func(t *testing.T) {
		broken := func() (domain.UserIdentity, error) { return domain.UserIdentity{}, errors.New("connection lost") }
		got := run(t, []func() (domain.UserIdentity, error){notLinked, broken}, domain.ErrEmailTaken, nil, active)
		if got.Outcome != domain.OutcomeFailed {
			t.Fatalf("結果 = %+v, want failed", got)
		}
	})
}

// TestGoogleLoginsCompleteWithoutFlow は、進行中の手続き(手続きを始めたブラウザの cookie に封じた値)がない要求では、
// 何も保存せずに、エラーを返すことを固定する(手続きを始めていない要求で、DB に書き込ませない)。
func TestGoogleLoginsCompleteWithoutFlow(t *testing.T) {
	handoffs := &fakeHandoffRepo{}
	logins := usecase.NewGoogleLogins(fakeGoogleProvider{}, &fakeIdentityQuery{lookups: []func() (domain.UserIdentity, error){notLinked}}, &fakeUserQuery{},
		&uowtest.UoW{}, domain.NewLoginHandoffs(handoffs), domain.NewUserIdentities(fakeIdentityRepo{}), fakeIssuer{})

	_, err := logins.Complete(context.Background(), usecase.GoogleFlow{}, usecase.GoogleCallback{Code: "c", State: "s"})

	if !errors.Is(err, usecase.ErrGoogleFlowMissing) {
		t.Fatalf("err = %v, want ErrGoogleFlowMissing", err)
	}
	if len(handoffs.created) != 0 {
		t.Fatalf("保存された結果 = %d 件, want 0", len(handoffs.created))
	}
}

// fakePendingHandoff は、画面へ渡すコードの中身の読み取り(usecase.LoginHandoffQuery)の代役である。
type fakePendingHandoff struct{ handoff domain.LoginHandoff }

func (f fakePendingHandoff) GetLoginHandoffByCodeHash(context.Context, string) (domain.LoginHandoff, error) {
	return f.handoff, nil
}

// TestGoogleLoginsRedeemRequiresBinder は、コードの交換が、手続きを終えたブラウザだけが持つ「結び付けの値」と一緒でなければ
// できず、合わない交換では、コードが消費されない(コードごと取り消される)ことを固定する。
func TestGoogleLoginsRedeemRequiresBinder(t *testing.T) {
	const binder = "binder-of-the-browser"
	redeem := func(t *testing.T, presented string) (domain.LoginHandoff, error, *fakeHandoffRepo, *uowtest.UoW) {
		t.Helper()
		handoffs := &fakeHandoffRepo{}
		stored := domain.LoginHandoff{ID: uid.N(1), Outcome: domain.OutcomeFailed, ReturnTo: "/oauth/authorize?x=1", BinderHash: domain.HashLoginHandoffBinder(binder)}
		unit := &uowtest.UoW{LoginHandoffs: handoffs, PendingHandoff: fakePendingHandoff{handoff: stored}}
		logins := usecase.NewGoogleLogins(fakeGoogleProvider{}, &fakeIdentityQuery{lookups: []func() (domain.UserIdentity, error){notLinked}}, &fakeUserQuery{},
			unit, domain.NewLoginHandoffs(handoffs), domain.NewUserIdentities(fakeIdentityRepo{}), fakeIssuer{})
		res, err := logins.Redeem(context.Background(), "the-code", presented)
		return domain.LoginHandoff{Outcome: res.Outcome, ReturnTo: res.ReturnTo}, err, handoffs, unit
	}

	t.Run("合う結び付けの値なら、結果(と、失敗のときの戻り先)を返し、コードを削除して確定する", func(t *testing.T) {
		got, err, handoffs, unit := redeem(t, binder)
		if err != nil || got.Outcome != domain.OutcomeFailed || got.ReturnTo != "/oauth/authorize?x=1" {
			t.Fatalf("結果 = %+v, err = %v", got, err)
		}
		if len(handoffs.discarded) != 1 || unit.Commits != 1 || unit.Rollbacks != 0 {
			t.Fatalf("削除 %v・commit %d・rollback %d, want 削除 1 件・commit 1", handoffs.discarded, unit.Commits, unit.Rollbacks)
		}
	})

	t.Run("空・合わない結び付けの値なら、無効として断り、コードを削除せず、全体を取り消す(コードは消費されない)", func(t *testing.T) {
		for name, presented := range map[string]string{"空": "", "別の値": "another-binder", "コードそのもの": "the-code"} {
			_, err, handoffs, unit := redeem(t, presented)
			if !errors.Is(err, domain.ErrLoginHandoffInvalid) {
				t.Errorf("%s: err = %v, want ErrLoginHandoffInvalid", name, err)
			}
			if len(handoffs.discarded) != 0 || unit.Commits != 0 || unit.Rollbacks != 1 {
				t.Errorf("%s: 削除 %v・commit %d・rollback %d, want 削除なし・rollback 1", name, handoffs.discarded, unit.Commits, unit.Rollbacks)
			}
		}
	})
}
