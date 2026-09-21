// Package uowtest は、usecase のテストで UnitOfWork を DB なしに組み立てるための test double を
// 提供する。usecase.UnitOfWork のフェイクは、渡された repository のフェイクを domain の書き込み
// オブジェクトで包んで Tx を作り、fn が成功すれば commit、エラーなら rollback を記録する。
// burger の統計は Stats が受け止め、ロックと保存の呼び出し順を記録する。
package uowtest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// FixedTime は、Clock の既定の時刻である。
var FixedTime = time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

// Clock は、固定の時刻を返す usecase.Clock である。T が零値なら FixedTime を返す。
type Clock struct{ T time.Time }

// Now は固定の時刻を返す。
func (c Clock) Now() time.Time {
	if c.T.IsZero() {
		return FixedTime
	}
	return c.T
}

// Stats は、domain.BurgerStatRepository と usecase.BurgerStatsQuery のフェイクである。
// 呼び出しを Ops に "lock:<id>" "facts:<id>" "save:<id>" "reviewed-by:<user>" "request:<id>"
// "complete:<id>@<version>" "failure:<id>@<version>" "due" の形で順に記録する。
// Facts と ReviewedBy と Due が nil のときは、facts も対象の burger も再計算の依頼も空を返す。
type Stats struct {
	mu sync.Mutex
	// Ops は呼び出しの記録である（順序つき）。
	Ops []string
	// Saved は保存された統計である（順序つき）。
	Saved []domain.BurgerStat
	// Facts は ListBurgerReviewFacts の振る舞いである。
	Facts func(ctx context.Context, burgerID string) ([]domain.ReviewFact, error)
	// ReviewedBy は ListReviewedBurgerIDsByUser の振る舞いである。
	ReviewedBy func(ctx context.Context, userID string) ([]string, error)
	// Due は ListDueRecalcRequests の振る舞い(再計算の時期が来ている依頼の一覧)である。
	Due func(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.RecalcRequest, error)
	// Failures は、記録された再計算の失敗である(順序つき)。
	Failures []RecordedFailure
	// LockErr と SaveErr は、ロックと保存のエラーである。
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

// Note は、テストが repository のフェイクなどから、呼び出しの順序の記録に印を足すためのものである
// （例: insert がロックの後にあることを確かめる）。
func (s *Stats) Note(op string) { s.record(op) }

// LockBurgerStat は、ロックの呼び出しを記録する。
func (s *Stats) LockBurgerStat(_ context.Context, burgerID string) error {
	s.record("lock:" + burgerID)
	return s.LockErr
}

// UpdateBurgerStat は、保存された統計を記録する。
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

// ListBurgerReviewFacts は、Facts があればそれを返し、なければ空を返す。
func (s *Stats) ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error) {
	s.record("facts:" + burgerID)
	if s.Facts == nil {
		return nil, nil
	}
	return s.Facts(ctx, burgerID)
}

// ListReviewedBurgerIDsByUser は、ReviewedBy があればそれを返し、なければ空を返す。
func (s *Stats) ListReviewedBurgerIDsByUser(ctx context.Context, userID string) ([]string, error) {
	s.record("reviewed-by:" + userID)
	if s.ReviewedBy == nil {
		return nil, nil
	}
	return s.ReviewedBy(ctx, userID)
}

// UoW は usecase.UnitOfWork のフェイクである。Reviews と Users の repository（フェイク）を、
// 呼ばれたときの Tx の書き込みオブジェクトにする。nil のものは、使われると panic する
// （想定外の書き込みに対して fail-loud する）。Stats が nil のときは、空の Stats を使う。
type UoW struct {
	Reviews domain.ReviewRepository
	Users   domain.UserRepository
	Stats   *Stats
	// BeginErr は、Do が開始できないエラー、CommitErr は commit のエラーである。
	BeginErr, CommitErr error
	// Commits と Rollbacks は、fn の結果に応じて数える。
	Commits, Rollbacks int
}

var _ usecase.UnitOfWork = (*UoW)(nil)

// Do は fn を実行し、成功なら commit、エラーなら rollback を記録する。
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
