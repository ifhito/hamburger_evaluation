package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ReviewQuery は、sqlc 生成のクエリ上で usecase.ReviewQuery を実装する。
// soft delete の述語（discarded_at IS NULL）と、active な shop に対する
// EXISTS フィルタは SQL 側にあり、認可ルール自体は domain パッケージにある。
type ReviewQuery struct {
	q *sqlcgen.Queries
}

// NewReviewQuery は db（通常は共有の pgx pool）をラップする。
func NewReviewQuery(db sqlcgen.DBTX) *ReviewQuery {
	return &ReviewQuery{q: sqlcgen.New(db)}
}

var _ usecase.ReviewQuery = (*ReviewQuery)(nil)

// ListReviews は公開 review フィードを返す。対象は、discard されておらず、
// かつ author（user）も discard されていない review のうち、burger が少なくとも
// 1 つの active な shop に紐づいているものに限る。filter で絞り込まれ、その意味は
// Rails の ReviewQuery に対応する（ただし shop の絞り込みは、対象の shop 自身も
// active であることを要求する点で Rails より厳しい）。filter.UserID・filter.BurgerID（本 API の
// 拡張）は、それぞれ、その user が書いた review・その burger の review に絞り込むだけで、
// 上記の公開ルールは一切迂回しない。author、burger、stats を 1 回のクエリで取得し（N+1 なし）、
// 新しい順（created_at desc、id desc）に並ぶ。
// keyword は likeEscaper を通して ILIKE パラメータに渡され、SQL に連結される
// ことはない。指定のない filter は NULL のままである。
// 次のページの有無を知るために limit+1 件を取得し、limit 件に切り詰めて返す。
// 2 つ目の戻り値は、offset+limit 件より後ろにも一致する review があるか（has_more）である。
func (r *ReviewQuery) ListReviews(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, bool, error) {
	params := sqlcgen.ListPublicReviewsParams{
		PageLimit:  limit + 1,
		PageOffset: offset,
	}
	if filter.Rating != nil {
		params.FilterRating = pgtype.Int8{Int64: int64(*filter.Rating), Valid: true}
	}
	if filter.Keyword != "" {
		params.CommentPattern = pgtype.Text{String: "%" + likeEscaper.Replace(filter.Keyword) + "%", Valid: true}
	}
	params.FilterShopID = filter.ShopID
	params.FilterUserID = filter.UserID
	params.FilterBurgerID = filter.BurgerID
	rows, err := r.q.ListPublicReviews(ctx, params)
	if err != nil {
		return nil, false, fmt.Errorf("list reviews: %w", err)
	}
	rows, hasMore := trimPage(rows, limit)
	reviews := make([]domain.ReviewDetail, 0, len(rows))
	for _, row := range rows {
		reviews = append(reviews, toReviewDetail(
			row.ID, row.Rating, row.Comment, row.PhotoKey, row.CreatedAt, row.VisitedAt,
			row.UserID, row.UserUsername, row.BurgerID, row.BurgerName,
			row.ReviewCount, row.AverageRating, row.WeightedScore, row.Confidence,
		))
	}
	return reviews, hasMore, nil
}

// GetReview は、discard されていない review 1 件を author、burger、stats
// とともに返す。または domain.ErrReviewNotFound を返す。author（user）が
// discard 済みの review も対象外である。SQL は、存在しない行、discard 済みの
// review、author が discard 済みの review をすべて同一に扱う。
func (r *ReviewQuery) GetReview(ctx context.Context, id string) (domain.ReviewDetail, error) {
	row, err := r.q.GetReviewDetail(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ReviewDetail{}, fmt.Errorf("get review: %w", domain.ErrReviewNotFound)
		}
		return domain.ReviewDetail{}, fmt.Errorf("get review: %w", err)
	}
	return toReviewDetail(
		row.ID, row.Rating, row.Comment, row.PhotoKey, row.CreatedAt, row.VisitedAt,
		row.UserID, row.UserUsername, row.BurgerID, row.BurgerName,
		row.ReviewCount, row.AverageRating, row.WeightedScore, row.Confidence,
	), nil
}

// GetShop は素の shop 行を返す。または domain.ErrShopNotFound を返す。
func (r *ReviewQuery) GetShop(ctx context.Context, id string) (domain.Shop, error) {
	row, err := r.q.GetShop(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Shop{}, fmt.Errorf("get shop: %w", domain.ErrShopNotFound)
		}
		return domain.Shop{}, fmt.Errorf("get shop: %w", err)
	}
	shop, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("get shop: %w", err)
	}
	return shop, nil
}

// ListReviewShops は、review が属する shop(review の burger を持つ shop すべて)を、作成の古い順に返す。
// 存在しない review・削除済みの review・shop に紐づかない burger の review は、空の一覧(エラーではない)を返す。
// どの shop を代表にするかは、ここでは決めない(domain.ReviewShopFor が決める)。
func (r *ReviewQuery) ListReviewShops(ctx context.Context, reviewID string) ([]domain.Shop, error) {
	rows, err := r.q.ListReviewShops(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("list review shops: %w", err)
	}
	shops := make([]domain.Shop, 0, len(rows))
	for _, row := range rows {
		shop, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("list review shops: %w", err)
		}
		shops = append(shops, shop)
	}
	return shops, nil
}

// GetShopBurger は、burger が shops_burgers 経由でその shop に紐づいている
// とき、stats つきの burger を返す。そうでなければ domain.ErrBurgerNotFound
// を返す。存在しない burger と別の shop の burger は区別できない。
func (r *ReviewQuery) GetShopBurger(ctx context.Context, shopID, burgerID string) (domain.ShopReviewBurger, error) {
	row, err := r.q.GetShopBurgerWithStats(ctx, sqlcgen.GetShopBurgerWithStatsParams{
		ShopID:   shopID,
		BurgerID: burgerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ShopReviewBurger{}, fmt.Errorf("get shop burger: %w", domain.ErrBurgerNotFound)
		}
		return domain.ShopReviewBurger{}, fmt.Errorf("get shop burger: %w", err)
	}
	return rowmap.ShopReviewBurger(row.ID, row.Name, row.AverageRating, row.ReviewCount, row.WeightedScore, row.Confidence), nil
}

// toReviewDetail は、結合された review のカラム（一覧クエリと詳細クエリで
// 共有される）を domain のペイロードに変換する。存在しない stats はゼロに
// なる。
func toReviewDetail(
	id string, rating int16, comment, photoKey pgtype.Text, createdAt pgtype.Timestamptz, visitedAt pgtype.Date,
	userID string, username string, burgerID string, burgerName string,
	reviewCount pgtype.Int8, averageRating, weightedScore, confidence pgtype.Float8,
) domain.ReviewDetail {
	burger := rowmap.ShopReviewBurger(burgerID, burgerName, averageRating, reviewCount, weightedScore, confidence)
	detail := domain.ReviewDetail{
		Review: domain.Review{
			ID:        id,
			Rating:    int(rating),
			AuthorID:  userID,
			BurgerID:  burgerID,
			CreatedAt: createdAt.Time,
			VisitedAt: rowmap.VisitedAt(visitedAt),
		},
		User:   &domain.UserRef{ID: userID, Username: username},
		Burger: &burger,
	}
	if comment.Valid {
		c := comment.String
		detail.Comment = &c
	}
	if photoKey.Valid {
		key := photoKey.String
		detail.PhotoKey = &key
	}
	return detail
}
