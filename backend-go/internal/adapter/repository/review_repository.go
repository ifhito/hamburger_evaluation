package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ReviewRepository implements usecase.ReviewRepository over sqlc-generated
// queries. The soft-delete predicate (discarded_at IS NULL) and the
// active-shop EXISTS filter live in the SQL; the authorization rules
// themselves live in the domain package.
type ReviewRepository struct {
	q *sqlcgen.Queries
}

// NewReviewRepository wraps db (normally the shared pgx pool).
func NewReviewRepository(db sqlcgen.DBTX) *ReviewRepository {
	return &ReviewRepository{q: sqlcgen.New(db)}
}

var _ usecase.ReviewRepository = (*ReviewRepository)(nil)

// ListReviews returns the public review feed: non-discarded reviews whose
// burger is linked to at least one active shop, with author, burger, and
// stats in a single query (no N+1), newest first.
func (r *ReviewRepository) ListReviews(ctx context.Context, limit, offset int32) ([]domain.ReviewDetail, error) {
	rows, err := r.q.ListPublicReviews(ctx, sqlcgen.ListPublicReviewsParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	reviews := make([]domain.ReviewDetail, 0, len(rows))
	for _, row := range rows {
		reviews = append(reviews, toReviewDetail(
			row.ID, row.Rating, row.Comment, row.CreatedAt,
			row.UserID, row.UserUsername, row.BurgerID, row.BurgerName,
			row.ReviewCount, row.AverageRating, row.WeightedScore, row.Confidence,
		))
	}
	return reviews, nil
}

// GetReview returns one non-discarded review with author, burger, and
// stats, or domain.ErrReviewNotFound — the SQL treats missing and
// discarded rows identically.
func (r *ReviewRepository) GetReview(ctx context.Context, id int64) (domain.ReviewDetail, error) {
	row, err := r.q.GetReviewDetail(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ReviewDetail{}, fmt.Errorf("get review: %w", domain.ErrReviewNotFound)
		}
		return domain.ReviewDetail{}, fmt.Errorf("get review: %w", err)
	}
	return toReviewDetail(
		row.ID, row.Rating, row.Comment, row.CreatedAt,
		row.UserID, row.UserUsername, row.BurgerID, row.BurgerName,
		row.ReviewCount, row.AverageRating, row.WeightedScore, row.Confidence,
	), nil
}

// GetShop returns the bare shop row, or domain.ErrShopNotFound.
func (r *ReviewRepository) GetShop(ctx context.Context, id int64) (domain.Shop, error) {
	row, err := r.q.GetShop(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Shop{}, fmt.Errorf("get shop: %w", domain.ErrShopNotFound)
		}
		return domain.Shop{}, fmt.Errorf("get shop: %w", err)
	}
	shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("get shop: %w", err)
	}
	return shop, nil
}

// GetShopBurger returns the burger with its stats when it is linked to
// the shop via shops_burgers, or domain.ErrBurgerNotFound — an unknown
// burger and a burger of another shop are indistinguishable.
func (r *ReviewRepository) GetShopBurger(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
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
	return domain.ShopReviewBurger{
		ID:   row.ID,
		Name: row.Name,
		// The stats row may not exist yet; zero values then.
		AverageRating: row.AverageRating.Float64,
		ReviewCount:   row.ReviewCount.Int64,
		WeightedScore: row.WeightedScore.Float64,
		Confidence:    row.Confidence.Float64,
	}, nil
}

// CreateReview inserts the (already validated) review and returns it with
// its generated id and created_at.
func (r *ReviewRepository) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	row, err := r.q.CreateReview(ctx, sqlcgen.CreateReviewParams{
		Rating:   int16(review.Rating),
		Comment:  textOrNull(review.Comment),
		UserID:   review.AuthorID,
		BurgerID: review.BurgerID,
	})
	if err != nil {
		return domain.Review{}, fmt.Errorf("create review: %w", err)
	}
	return toDomainReview(row), nil
}

// UpdateReviewContent persists only rating and comment of the still kept
// review under id and returns the stored row, or domain.ErrReviewNotFound
// when it is missing or discarded. Column-scoped: discarded_at is never
// written, so an edit can neither resurrect nor race a soft delete.
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

// DiscardReview soft-deletes the review (stamps discarded_at, never a
// hard DELETE). Missing and already-discarded reviews match no row and
// yield domain.ErrReviewNotFound.
func (r *ReviewRepository) DiscardReview(ctx context.Context, id int64) error {
	affected, err := r.q.DiscardReview(ctx, id)
	if err != nil {
		return fmt.Errorf("discard review: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("discard review: %w", domain.ErrReviewNotFound)
	}
	return nil
}

// toDomainReview maps a sqlc review row onto the domain entity.
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
	return review
}

// toReviewDetail maps the joined review columns (shared by the list and
// detail queries) onto the domain payload; absent stats become zeros.
func toReviewDetail(
	id int64, rating int16, comment pgtype.Text, createdAt pgtype.Timestamptz,
	userID int64, username string, burgerID int64, burgerName string,
	reviewCount pgtype.Int8, averageRating, weightedScore, confidence pgtype.Float8,
) domain.ReviewDetail {
	detail := domain.ReviewDetail{
		Review: domain.Review{
			ID:        id,
			Rating:    int(rating),
			AuthorID:  userID,
			BurgerID:  burgerID,
			CreatedAt: createdAt.Time,
		},
		User: &domain.UserRef{ID: userID, Username: username},
		Burger: &domain.ShopReviewBurger{
			ID:   burgerID,
			Name: burgerName,
			// The stats row may not exist yet; zero values then.
			AverageRating: averageRating.Float64,
			ReviewCount:   reviewCount.Int64,
			WeightedScore: weightedScore.Float64,
			Confidence:    confidence.Float64,
		},
	}
	if comment.Valid {
		c := comment.String
		detail.Comment = &c
	}
	return detail
}
