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

// ShopQuery は、sqlc 生成のクエリ上で usecase.ShopQuery を実装する。
// domain の visibility 記述子を SQL パラメータに変換し、smallint の status
// コードを domain.ShopStatus に対応づける。visibility のルール自体は
// domain.ShopVisibility にある。
type ShopQuery struct {
	q *sqlcgen.Queries
}

// NewShopQuery は db（通常は共有の pgx pool）をラップする。
func NewShopQuery(db sqlcgen.DBTX) *ShopQuery {
	return &ShopQuery{q: sqlcgen.New(db)}
}

var _ usecase.ShopQuery = (*ShopQuery)(nil)

// ListShops は、keyword に一致する可視の shop を name、id の順に並べて、集計(shop_stats の保存された値。
// まだ集計されていないショップは空の集計)つきで返す。集計は LEFT JOIN で添えるので、クエリは 1 回である。
// keyword はエスケープ済みの ILIKE パラメータとして渡され、SQL に連結される
// ことはない。次のページの有無を知るために limit+1 件を取得し、limit 件に切り詰めて
// 返す。2 つ目の戻り値は、offset+limit 件より後ろにも見える shop があるか（has_more）である。
func (r *ShopQuery) ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.ShopListing, bool, error) {
	params := sqlcgen.ListShopsParams{
		ViewAll:    vis.ViewAll,
		PageLimit:  limit + 1,
		PageOffset: offset,
	}
	params.ViewerID = vis.ViewerID
	if keyword != "" {
		params.NamePattern = pgtype.Text{String: "%" + likeEscaper.Replace(keyword) + "%", Valid: true}
	}
	rows, err := r.q.ListShops(ctx, params)
	if err != nil {
		return nil, false, fmt.Errorf("list shops: %w", err)
	}
	rows, hasMore := trimPage(rows, limit)
	listings := make([]domain.ShopListing, 0, len(rows))
	for _, row := range rows {
		shop, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, false, fmt.Errorf("list shops: %w", err)
		}
		listings = append(listings, domain.ShopListing{Shop: shop, Summary: rowmap.ShopSummary(row.ReviewCount, row.AverageRating, row.PhotoKey)})
	}
	return listings, hasMore, nil
}

// GetShopWithCreator は shop とその creator を、集計(shop_stats の保存された値)つきで返す（Reviews は空のまま）。
// または domain.ErrShopNotFound を返す。
func (r *ShopQuery) GetShopWithCreator(ctx context.Context, id string) (domain.ShopDetail, error) {
	row, err := r.q.GetShopWithCreator(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", domain.ErrShopNotFound)
		}
		return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", err)
	}
	shop, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", err)
	}
	detail := domain.ShopDetail{Shop: shop, Summary: rowmap.ShopSummary(row.ReviewCount, row.AverageRating, row.PhotoKey)}
	if row.CreatorID != nil {
		// users.id は外部キーなので、LEFT JOIN で creator が見つかっている。
		detail.Creator = &domain.UserRef{ID: *row.CreatorID, Username: row.CreatorUsername.String}
	}
	return detail, nil
}

// ListShopReviews は、shop に紐づく burger の、discard されていない review の
// うち author（user）も discard されていないものを、author、burger、stats
// とともに新しい順（created_at desc、id desc）に返す。discard 済みの user の
// （まだ kept な）review は含まれない。単一の JOIN クエリである（N+1 なし）。
func (r *ShopQuery) ListShopReviews(ctx context.Context, shopID string) ([]domain.ShopReview, error) {
	rows, err := r.q.ListShopReviews(ctx, shopID)
	if err != nil {
		return nil, fmt.Errorf("list shop reviews: %w", err)
	}
	reviews := make([]domain.ShopReview, 0, len(rows))
	for _, row := range rows {
		burger := rowmap.ShopReviewBurger(row.BurgerID, row.BurgerName, row.AverageRating, row.ReviewCount, row.WeightedScore, row.Confidence)
		review := domain.ShopReview{
			ID:        row.ID,
			Rating:    int(row.Rating),
			CreatedAt: row.CreatedAt.Time,
			User:      &domain.UserRef{ID: row.UserID, Username: row.UserUsername},
			Burger:    &burger,
		}
		if row.Comment.Valid {
			comment := row.Comment.String
			review.Comment = &comment
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

// ListShopsForModeration は、すべての shop をその creator とともに新しい順
// （created_at desc、id desc）に返す。任意で 1 つの status に絞り込める。
func (r *ShopQuery) ListShopsForModeration(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
	var filter pgtype.Int2
	if status != nil {
		code, err := rowmap.ShopStatusCode(*status)
		if err != nil {
			return nil, fmt.Errorf("list shops for moderation: %w", err)
		}
		filter = pgtype.Int2{Int16: code, Valid: true}
	}
	rows, err := r.q.ListShopsForModeration(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list shops for moderation: %w", err)
	}
	details := make([]domain.ShopDetail, 0, len(rows))
	for _, row := range rows {
		shop, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("list shops for moderation: %w", err)
		}
		detail := domain.ShopDetail{Shop: shop}
		if row.CreatorID != nil {
			// users.id は外部キーなので、LEFT JOIN で creator が見つかっている。
			detail.Creator = &domain.UserRef{ID: *row.CreatorID, Username: row.CreatorUsername.String}
		}
		details = append(details, detail)
	}
	return details, nil
}
