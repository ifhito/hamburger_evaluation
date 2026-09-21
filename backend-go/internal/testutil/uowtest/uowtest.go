// Package uowtest は、usecase のテストで、データベースなしに UnitOfWork を使えるようにする、
// テスト用の代役(フェイク)を提供する。UnitOfWork(作業のひとまとまり)は、ここからここまでの
// 書き込みと読み取りを、まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組みで、
// 途中でエラーになれば全体を取り消す(rollback)。
//
// UoW は、渡された repository の代役を domain の書き込みオブジェクトで包んで Tx を作り、fn が成功
// すれば commit、エラーを返せば rollback したものとして回数を数える。バーガーの統計に対する操作
// (ロック・元データの読み取り・保存)は Stats が受け止め、呼ばれた順に記録する。本物の
// トランザクションの隔離やロックの効き目は、実データベースを使うテスト(adapter/uow)で確かめる。
// 本番のコードから import してはならない。
package uowtest

import (
	"context"
	"sync"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// FixedTime は、Clock の既定の時刻である(テストの結果が実行時刻に左右されないようにする)。
var FixedTime = time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

// Clock は、常に同じ時刻を返す usecase.Clock である。T が未設定(ゼロ値)なら FixedTime を返す。
type Clock struct{ T time.Time }

// Now は固定の時刻を返す。
func (c Clock) Now() time.Time {
	if c.T.IsZero() {
		return FixedTime
	}
	return c.T
}

// Stats は、domain.BurgerStatRepository(統計のロックと保存)と usecase.BurgerStatsQuery(統計の
// 元データの読み取り)の、両方を兼ねる代役である。呼び出しを、Ops に次の形で呼ばれた順に記録する。
//
//	"lock:<バーガー ID>"        バーガーの行のロック
//	"facts:<バーガー ID>"       バーガーの統計の元データの読み取り
//	"save:<バーガー ID>"        統計の保存
//	"reviewed-by:<ユーザー ID>" ユーザーがレビューしたバーガーの一覧の読み取り
//
// Facts と ReviewedBy が未設定(nil)のときは、元データも対象のバーガーも空として返す。
type Stats struct {
	mu sync.Mutex
	// Ops は、上の形式で記録した操作の並びである(呼ばれた順)。
	Ops []string
	// Saved は、保存された統計である(保存された順)。
	Saved []domain.BurgerStat
	// Facts は、統計の元データの読み取り(ListBurgerReviewFacts)が返す内容を決める。
	Facts func(ctx context.Context, burgerID string) ([]domain.ReviewFact, error)
	// ReviewedBy は、ユーザーがレビューしたバーガーの一覧の読み取り(ListReviewedBurgerIDsByUser)が
	// 返す内容を決める。
	ReviewedBy func(ctx context.Context, userID string) ([]string, error)
	// LockErr と SaveErr を設定すると、それぞれロックと保存がそのエラーで失敗する。
	LockErr, SaveErr error
}

var (
	_ domain.BurgerStatRepository = (*Stats)(nil)
	_ usecase.BurgerStatsQuery    = (*Stats)(nil)
)

func (s *Stats) record(op string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Ops = append(s.Ops, op)
}

// Note は、テストが別の代役(レビューの登録など)から、操作の記録に印を足すためのものである。
// 統計に対する操作と、レビューに対する操作の前後関係(登録の前にロックしているか、など)を、
// 1 つの並びで確かめられる。
func (s *Stats) Note(op string) { s.record(op) }

// LockBurgerStat は、ロックの呼び出しを記録する(LockErr があればそのエラーで失敗する)。
func (s *Stats) LockBurgerStat(_ context.Context, burgerID string) error {
	s.record("lock:" + burgerID)
	return s.LockErr
}

// UpdateBurgerStat は、保存の呼び出しと、保存された統計を記録する(SaveErr があればそのエラーで
// 失敗し、統計は記録しない)。
func (s *Stats) UpdateBurgerStat(_ context.Context, stat domain.BurgerStat) error {
	s.record("save:" + stat.BurgerID)
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Saved = append(s.Saved, stat)
	return nil
}

// ListBurgerReviewFacts は、Facts があればその結果を、なければ空を返す。
func (s *Stats) ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error) {
	s.record("facts:" + burgerID)
	if s.Facts == nil {
		return nil, nil
	}
	return s.Facts(ctx, burgerID)
}

// ListReviewedBurgerIDsByUser は、ReviewedBy があればその結果を、なければ空を返す。
func (s *Stats) ListReviewedBurgerIDsByUser(ctx context.Context, userID string) ([]string, error) {
	s.record("reviewed-by:" + userID)
	if s.ReviewedBy == nil {
		return nil, nil
	}
	return s.ReviewedBy(ctx, userID)
}

// UoW は usecase.UnitOfWork(まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組み)の
// 代役である。名前は Unit of Work の略。Reviews と Users に渡した repository の代役を、Do が
// 作る Tx の書き込みオブジェクトにする。未設定(nil)のものが使われると panic するので、テストが
// 想定していない書き込みは、黙って通らずに失敗する。Stats が未設定なら、空の Stats を使う。
type UoW struct {
	Reviews domain.ReviewRepository
	Users   domain.UserRepository
	Stats   *Stats
	// BeginErr を設定すると、Do はトランザクションを開始できずにそのエラーを返す。CommitErr を
	// 設定すると、fn が成功しても commit に失敗してそのエラーを返す(rollback として数える)。
	BeginErr, CommitErr error
	// Commits と Rollbacks は、commit と rollback になった回数である。
	Commits, Rollbacks int
}

var _ usecase.UnitOfWork = (*UoW)(nil)

// Do は fn を実行し、成功なら commit、エラーなら rollback として回数を数える。
func (u *UoW) Do(ctx context.Context, fn func(ctx context.Context, tx usecase.Tx) error) error {
	if u.BeginErr != nil {
		return u.BeginErr
	}
	if u.Stats == nil {
		u.Stats = &Stats{}
	}
	tx := usecase.Tx{
		Reviews:     domain.NewReviews(u.Reviews),
		Users:       domain.NewUsers(u.Users),
		BurgerStats: domain.NewBurgerStats(u.Stats),
		Stats:       u.Stats,
	}
	if err := fn(ctx, tx); err != nil {
		u.Rollbacks++
		return err
	}
	if u.CommitErr != nil {
		u.Rollbacks++
		return u.CommitErr
	}
	u.Commits++
	return nil
}
