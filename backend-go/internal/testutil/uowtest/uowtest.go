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
	"fmt"
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

// Stats は、domain.BurgerStatRepository(統計のロック・保存・再計算の依頼)と usecase.BurgerStatsQuery(統計の
// 元データの読み取りと、再計算の依頼の一覧)の、両方を兼ねる代役である。呼び出しを、Ops に次の形で
// 呼ばれた順に記録する。
//
//	"lock:<バーガー ID>"              バーガーの行のロック
//	"facts:<バーガー ID>"             バーガーの統計の元データの読み取り
//	"save:<バーガー ID>"              統計の保存
//	"reviewed-by:<ユーザー ID>"       ユーザーがレビューしたバーガーの一覧の読み取り
//	"request:<バーガー ID>"           再計算の依頼の登録
//	"complete:<バーガー ID>@<version>" 再計算を終えた依頼の削除
//	"failure:<バーガー ID>@<version>"  再計算の失敗の記録
//	"due"                             再計算の時期が来ている依頼の一覧の読み取り
//
// Facts、ReviewedBy、Due が未設定(nil)のときは、元データも対象のバーガーも依頼も空として返す。
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
	// Due は ListDueRecalcRequests の振る舞い(再計算の時期が来ている依頼の一覧)である。
	Due func(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.RecalcRequest, error)
	// Failures は、記録された再計算の失敗である(順序つき)。
	Failures []RecordedFailure
	// LockErr と SaveErr を設定すると、それぞれロックと保存がそのエラーで失敗する。
	LockErr, SaveErr error
	// RequestErr は再計算の依頼の登録、CompleteErr は依頼の削除、FailureErr は失敗の記録のエラーである。
	RequestErr, CompleteErr, FailureErr error
	// CompleteRejected が true のとき、依頼の削除は「version が進んでいて消せなかった」(false)を返す。
	CompleteRejected bool
}

// RecordedFailure は、再計算の失敗として記録された内容である。
type RecordedFailure struct {
	BurgerID string
	Version  int64
	Failure  domain.RecalcFailure
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

// CreateBurgerStatRecalcRequest は、再計算の依頼の登録を記録する。
func (s *Stats) CreateBurgerStatRecalcRequest(_ context.Context, burgerID string) error {
	s.record("request:" + burgerID)
	return s.RequestErr
}

// DiscardBurgerStatRecalcRequest は、依頼の削除を記録する。CompleteRejected なら、消せなかった(false)を返す。
func (s *Stats) DiscardBurgerStatRecalcRequest(_ context.Context, burgerID string, version int64) (bool, error) {
	s.record(fmt.Sprintf("complete:%s@%d", burgerID, version))
	if s.CompleteErr != nil {
		return false, s.CompleteErr
	}
	return !s.CompleteRejected, nil
}

// UpdateBurgerStatRecalcFailure は、再計算の失敗の記録を保存する。
func (s *Stats) UpdateBurgerStatRecalcFailure(_ context.Context, burgerID string, version int64, failure domain.RecalcFailure) (bool, error) {
	s.record(fmt.Sprintf("failure:%s@%d", burgerID, version))
	if s.FailureErr != nil {
		return false, s.FailureErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Failures = append(s.Failures, RecordedFailure{BurgerID: burgerID, Version: version, Failure: failure})
	return true, nil
}

// ListDueRecalcRequests は、Due があればそれを返し、なければ空を返す。
func (s *Stats) ListDueRecalcRequests(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.RecalcRequest, error) {
	s.record("due")
	if s.Due == nil {
		return nil, nil
	}
	return s.Due(ctx, now, maxAttempts, batch)
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
