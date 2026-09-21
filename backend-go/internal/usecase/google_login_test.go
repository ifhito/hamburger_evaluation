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

// fakeHandoffRepo は、画面へ渡すコードの書き込み(domain.LoginHandoffRepository)の代役で、保存された中身を記録する。
type fakeHandoffRepo struct {
	created []domain.CreateLoginHandoffParams
}

func (f *fakeHandoffRepo) CreateLoginHandoff(_ context.Context, p domain.CreateLoginHandoffParams) error {
	f.created = append(f.created, p)
	return nil
}

func (*fakeHandoffRepo) LockLoginHandoff(context.Context, string) error {
	panic("unexpected LockLoginHandoff call")
}

func (*fakeHandoffRepo) DiscardLoginHandoff(context.Context, string) error {
	panic("unexpected DiscardLoginHandoff call")
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
