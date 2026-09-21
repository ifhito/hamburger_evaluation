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

// ReviewRepository は、sqlc が生成したクエリを使って domain.ReviewRepository を実装する。
// レビューの書き込み(登録・編集・論理削除)だけを担い、読み取りは adapter/query の ReviewQuery が
// 担う。論理削除済みの行を除く条件(discarded_at IS NULL)は SQL に書かれていて、編集・削除できる
// のは誰かという認可のルールは domain にある。
//
// バーガーの統計の再計算は、ここでは行わない。レビューの書き込みと同じトランザクションで、
// usecase が UnitOfWork(ここからここまでをまとめて 1 つのトランザクションにする範囲を、usecase が
// 指定する仕組み)の中で、BurgerStatRepository と組み合わせて行う。db が UnitOfWork のトランザクション
// のときは、この中で複数の文をまとめるための withTx が、新しいトランザクションではなくセーブポイント
// (トランザクションの途中に打つ、部分的な巻き戻し用の目印)になり、外側のトランザクションの中で
// 矛盾なく巻き戻る。
type ReviewRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewReviewRepository は db をラップする。本番では、共有の接続プールか、UnitOfWork の
// トランザクション(まとめて 1 つのトランザクションにする範囲の中で使うもの)が渡される。
func NewReviewRepository(db beginnerDBTX) *ReviewRepository {
	return &ReviewRepository{db: db, q: sqlcgen.New(db)}
}

var _ domain.ReviewRepository = (*ReviewRepository)(nil)

// CreateReview は、検証済みのレビューを登録し、採番された ID と作成日時を持つレビューを返す。
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

// CreateShopBurger は、ショップのバーガーのうち、名前が burgerName とちょうど一致するもの(前後の
// 空白や大文字小文字も区別する)を返す。そのショップに同名のバーガーがなければ、バーガーを作成して
// ショップと結び付ける(shops_burgers)。
//
// 返すバーガーの統計は、この呼び出しの前の値である(まったく新しいバーガーなら 0)。バーガー ID を
// 指定した投稿で、読み取り(GetShopBurger)が返す値と同じ意味になる。
//
// (ショップ, 名前)には一意制約を付けていない。同じ新しい名前で同時に投稿した 2 人が、それぞれ
// バーガーを作ることがある。名前の重複を許す仕様であって、不具合ではない。
//
// 検索から作成までの複数の文を 1 つにまとめるので、途中で失敗しても、作りかけのバーガーや結び付けが
// 残ることはない。ただし、その後のレビューの登録まで同じトランザクションにするのは、呼び出し側
// (UnitOfWork。まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組み)の役目である。
func (r *ReviewRepository) CreateShopBurger(ctx context.Context, shopID string, burgerName string) (domain.ShopReviewBurger, error) {
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

// UpdateReviewContent は、削除されていないレビューの評価とコメントだけを更新し、更新後の行を
// 返す。レビューが存在しない、または論理削除済みなら domain.ErrReviewNotFound を返す。更新する
// 列を絞っているので、編集が、論理削除の目印(discarded_at)を消して削除を取り消したり、同時に
// 行われた論理削除と食い違ったりすることはない。
func (r *ReviewRepository) UpdateReviewContent(ctx context.Context, id string, rating int, comment string) (domain.Review, error) {
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

// UpdateReviewContentAndPhotoKey は、削除されていないレビューの評価・コメント・写真のキーを更新し、
// 更新後の行を返す。レビューが存在しない、または論理削除済みなら domain.ErrReviewNotFound を返す。
// 評価とコメントの更新と、写真のキーの更新は、1 つのトランザクションで行う。途中で失敗しても、
// 写真のキーが付かないままコメントだけが確定することはない。写真のキーの更新が返す行には、
// 直前の更新(評価とコメント)がすでに反映されている。
func (r *ReviewRepository) UpdateReviewContentAndPhotoKey(ctx context.Context, id string, rating int, comment string, photoKey *string) (domain.Review, error) {
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

// DiscardReview はレビューを論理削除する(削除日時を記録するだけで、行は消さない)。レビューが
// 存在しない、またはすでに論理削除済みなら domain.ErrReviewNotFound を返す。
func (r *ReviewRepository) DiscardReview(ctx context.Context, id string) error {
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
