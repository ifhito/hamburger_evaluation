package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ReviewRepository は、sqlc 生成のクエリ上で domain.ReviewRepository を
// 実装する。書き込み（create、edit、discard）だけを担い、読み取りは
// adapter/query の ReviewQuery が担う。soft delete の述語（discarded_at IS NULL）は
// SQL 側にあり、認可ルール自体は domain パッケージにある。burger_stats の再計算は
// ここでは行わない（トランザクションを持つ usecase が、UnitOfWork の中で
// BurgerStatRepository と組み合わせて行う）。db が UnitOfWork のトランザクション
// （pgx.Tx）のときは、複数の文を 1 つにまとめる withTx は savepoint になり、
// 全体のトランザクションの中で原子的に働く。
type ReviewRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewReviewRepository は db（共有の pgx pool か、UnitOfWork のトランザクション）を
// ラップする。
func NewReviewRepository(db beginnerDBTX) *ReviewRepository {
	return &ReviewRepository{db: db, q: sqlcgen.New(db)}
}

var _ domain.ReviewRepository = (*ReviewRepository)(nil)

// CreateReview は（検証済みの）review を insert し、生成された id と
// created_at を持つ review を返す。
func (r *ReviewRepository) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	row, err := r.q.CreateReview(ctx, sqlcgen.CreateReviewParams{
		Rating:   int16(review.Rating),
		Comment:  textOrNull(review.Comment),
		UserID:   review.AuthorID,
		BurgerID: review.BurgerID,
		PhotoKey: textOrNull(review.PhotoKey),
	})
	if err != nil {
		return domain.Review{}, fmt.Errorf("create review: %w", err)
	}
	return toDomainReview(row), nil
}

// CreateShopBurger は、shop の burger のうち名前が burgerName と完全に一致するものを
// 返す。shop にその名前の burger がないときは、burger とその shops_burgers の link を
// 作成する（Rails の find_or_create_burger、S6 P3-1）。(shop, name) には unique index が
// 意図的に存在しない。同じ新しい名前を同時に作成する 2 つの creator が、両方とも burger を
// insert しうる。これは Rails の find_or_create_burger にもある同じ race であり、parity
// であってバグではない。返される burger は、この呼び出しの前の stats を持ち、burger_id
// 経路での GetShopBurger とまったく同じである（まったく新しい burger ではゼロ）。
// 複数の文を 1 つにまとめるので、途中で失敗しても、孤立した burger や link は残らない。
// review の insert までを同じトランザクションにするのは、呼び出し側（UnitOfWork）の責務である。
func (r *ReviewRepository) CreateShopBurger(ctx context.Context, shopID int64, burgerName string) (domain.ShopReviewBurger, error) {
	var burger domain.ShopReviewBurger
	err := withTx(ctx, r.db, "create shop burger", func(q *sqlcgen.Queries) error {
		found, err := q.GetShopBurgerByNameWithStats(ctx, sqlcgen.GetShopBurgerByNameWithStatsParams{
			ShopID: shopID,
			Name:   burgerName,
		})
		switch {
		case err == nil:
			burger = rowmap.ShopReviewBurger(found.ID, found.Name, found.AverageRating, found.ReviewCount, found.WeightedScore, found.Confidence)
		case errors.Is(err, pgx.ErrNoRows):
			created, err := q.CreateBurger(ctx, burgerName)
			if err != nil {
				return fmt.Errorf("create shop burger: create burger: %w", err)
			}
			if err := q.CreateShopBurger(ctx, sqlcgen.CreateShopBurgerParams{ShopID: shopID, BurgerID: created.ID}); err != nil {
				return fmt.Errorf("create shop burger: link burger: %w", err)
			}
			burger = domain.ShopReviewBurger{ID: created.ID, Name: created.Name}
		default:
			return fmt.Errorf("create shop burger: find burger: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.ShopReviewBurger{}, err
	}
	return burger, nil
}

// UpdateReviewContent は、id の、まだ kept な review の rating と comment
// だけを永続化し、保存された行を返す。存在しないか discard 済みの場合は
// domain.ErrReviewNotFound を返す。カラム単位に限定される：discarded_at は決して
// 書き込まれないので、edit が soft delete を復活させることも、soft delete と
// race することもない。
func (r *ReviewRepository) UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error) {
	row, err := r.q.UpdateReviewContent(ctx, sqlcgen.UpdateReviewContentParams{
		ID:      id,
		Rating:  int16(rating),
		Comment: pgtype.Text{String: comment, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Review{}, fmt.Errorf("update review content: %w", domain.ErrReviewNotFound)
		}
		return domain.Review{}, fmt.Errorf("update review content: %w", err)
	}
	return toDomainReview(row), nil
}

// UpdateReviewContentAndPhotoKey は、id の、まだ kept な review の rating、
// comment、photo_key を永続化し、存在しないか discard 済みの場合は
// domain.ErrReviewNotFound を返す。既存のカラム単位の 2 つのステートメント
// （UpdateReviewContent、続いて UpdateReviewPhotoKey）はただ 1 つのトランザクションで
// 実行されるので、photo を伴う edit は、content と key をまとめて commit するか、何も
// commit しないかのどちらかになる。2 番目のステートメントが返す行には、最初のステートメントの
// rating/comment がすでに反映されている（同一トランザクション）。
func (r *ReviewRepository) UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error) {
	var row sqlcgen.Review
	err := withTx(ctx, r.db, "update review content and photo key", func(q *sqlcgen.Queries) error {
		if _, err := q.UpdateReviewContent(ctx, sqlcgen.UpdateReviewContentParams{
			ID:      id,
			Rating:  int16(rating),
			Comment: pgtype.Text{String: comment, Valid: true},
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("update review content and photo key: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("update review content and photo key: %w", err)
		}
		var err error
		row, err = q.UpdateReviewPhotoKey(ctx, sqlcgen.UpdateReviewPhotoKeyParams{
			ID:       id,
			PhotoKey: textOrNull(photoKey),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("update review content and photo key: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("update review content and photo key: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Review{}, err
	}
	return toDomainReview(row), nil
}

// DiscardReview は review を soft delete する（discarded_at に時刻を刻み、
// hard DELETE は決して行わない）。存在しない review や、すでに discard 済みの
// review はどの行にも一致せず、domain.ErrReviewNotFound を返す。
func (r *ReviewRepository) DiscardReview(ctx context.Context, id int64) error {
	if _, err := r.q.DiscardReview(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("discard review: %w", domain.ErrReviewNotFound)
		}
		return fmt.Errorf("discard review: %w", err)
	}
	return nil
}

// toDomainReview は sqlc の review 行を domain のエンティティに変換する。
func toDomainReview(row sqlcgen.Review) domain.Review {
	review := domain.Review{
		ID:        row.ID,
		Rating:    int(row.Rating),
		AuthorID:  row.UserID,
		BurgerID:  row.BurgerID,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.Comment.Valid {
		comment := row.Comment.String
		review.Comment = &comment
	}
	if row.PhotoKey.Valid {
		key := row.PhotoKey.String
		review.PhotoKey = &key
	}
	return review
}
