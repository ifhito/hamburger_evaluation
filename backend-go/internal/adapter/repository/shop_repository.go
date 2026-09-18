package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ShopRepository implements usecase.ShopRepository over sqlc-generated
// queries. It translates the domain visibility descriptor into SQL
// parameters and maps the smallint status codes onto domain.ShopStatus;
// the visibility rule itself lives in domain.ShopVisibility.
type ShopRepository struct {
	q *sqlcgen.Queries
}

// NewShopRepository wraps db (normally the shared pgx pool).
func NewShopRepository(db sqlcgen.DBTX) *ShopRepository {
	return &ShopRepository{q: sqlcgen.New(db)}
}

var _ usecase.ShopRepository = (*ShopRepository)(nil)

// likeEscaper escapes the LIKE metacharacters (backslash first) so user
// keywords always match literally inside an ILIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ListShops returns visible shops matching keyword ordered by name, id.
// The keyword is passed as an escaped ILIKE parameter, never concatenated
// into SQL.
func (r *ShopRepository) ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error) {
	params := sqlcgen.ListShopsParams{
		ViewAll:    vis.ViewAll,
		PageLimit:  limit,
		PageOffset: offset,
	}
	if vis.ViewerID != nil {
		params.ViewerID = pgtype.Int8{Int64: *vis.ViewerID, Valid: true}
	}
	if keyword != "" {
		params.NamePattern = pgtype.Text{String: "%" + likeEscaper.Replace(keyword) + "%", Valid: true}
	}
	rows, err := r.q.ListShops(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list shops: %w", err)
	}
	shops := make([]domain.Shop, 0, len(rows))
	for _, row := range rows {
		shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("list shops: %w", err)
		}
		shops = append(shops, shop)
	}
	return shops, nil
}

// GetShopWithCreator returns the shop and its creator (Reviews left
// empty), or domain.ErrShopNotFound.
func (r *ShopRepository) GetShopWithCreator(ctx context.Context, id int64) (domain.ShopDetail, error) {
	row, err := r.q.GetShopWithCreator(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", domain.ErrShopNotFound)
		}
		return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", err)
	}
	shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", err)
	}
	detail := domain.ShopDetail{Shop: shop}
	if row.CreatorID.Valid {
		// users.id is a foreign key, so the LEFT JOIN found the creator.
		detail.Creator = &domain.UserRef{ID: row.CreatorID.Int64, Username: row.CreatorUsername.String}
	}
	return detail, nil
}

// ListShopReviews returns the shop's non-discarded reviews with author,
// burger, and stats, newest first — a single JOIN query (no N+1).
func (r *ShopRepository) ListShopReviews(ctx context.Context, shopID int64) ([]domain.ShopReview, error) {
	rows, err := r.q.ListShopReviews(ctx, shopID)
	if err != nil {
		return nil, fmt.Errorf("list shop reviews: %w", err)
	}
	reviews := make([]domain.ShopReview, 0, len(rows))
	for _, row := range rows {
		review := domain.ShopReview{
			ID:        row.ID,
			Rating:    int(row.Rating),
			CreatedAt: row.CreatedAt.Time,
			User:      &domain.UserRef{ID: row.UserID, Username: row.UserUsername},
			Burger: &domain.ShopReviewBurger{
				ID:   row.BurgerID,
				Name: row.BurgerName,
				// The stats row may not exist yet; zero values then.
				AverageRating: row.AverageRating.Float64,
				ReviewCount:   row.ReviewCount.Int64,
				WeightedScore: row.WeightedScore.Float64,
				Confidence:    row.Confidence.Float64,
			},
		}
		if row.Comment.Valid {
			comment := row.Comment.String
			review.Comment = &comment
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

// toDomainShop maps sqlc shop columns onto the domain entity, decoding
// the smallint status (0=pending, 1=active, 2=rejected).
func toDomainShop(id int64, name string, status int16, note pgtype.Text, creatorID pgtype.Int8) (domain.Shop, error) {
	shop := domain.Shop{ID: id, Name: name}
	switch status {
	case 0:
		shop.Status = domain.ShopStatusPending
	case 1:
		shop.Status = domain.ShopStatusActive
	case 2:
		shop.Status = domain.ShopStatusRejected
	default:
		// Unreachable while the CHECK constraint holds; fail loudly if not.
		return domain.Shop{}, fmt.Errorf("shop %d: unknown status code %d", id, status)
	}
	if note.Valid {
		n := note.String
		shop.ModerationNote = &n
	}
	if creatorID.Valid {
		c := creatorID.Int64
		shop.CreatorID = &c
	}
	return shop, nil
}
