package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// Pagination bounds for the global review feed (same Rails parity as
// shops).
const (
	defaultReviewsPerPage = 20
	maxReviewsPerPage     = 100
)

// ReviewRepository is the consumer-side persistence contract for reviews.
// Implementations keep the SQL details (EXISTS active-shop filter, the
// smallint status encoding, soft-delete predicates) to themselves and
// return wrapped domain sentinels (ErrReviewNotFound, ErrShopNotFound,
// ErrBurgerNotFound) when no row matches.
type ReviewRepository interface {
	// ListReviews returns the non-discarded reviews whose burger is
	// served by at least one active shop, with author, burger, and stats
	// joined (no N+1), newest first (created_at desc, id desc).
	ListReviews(ctx context.Context, limit, offset int32) ([]domain.ReviewDetail, error)
	// GetReview returns one non-discarded review with author, burger, and
	// stats, or (a wrapped) domain.ErrReviewNotFound — missing and
	// discarded reviews are indistinguishable.
	GetReview(ctx context.Context, id int64) (domain.ReviewDetail, error)
	// GetShop returns the bare shop row (no creator, no reviews) or (a
	// wrapped) domain.ErrShopNotFound.
	GetShop(ctx context.Context, id int64) (domain.Shop, error)
	// GetShopBurger returns the burger with its stats (zeros when none
	// calculated yet) only when it is linked to the shop via
	// shops_burgers, else (a wrapped) domain.ErrBurgerNotFound.
	GetShopBurger(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error)
	// CreateReview persists a new (already validated) review and returns
	// it with its generated id and created_at.
	CreateReview(ctx context.Context, review domain.Review) (domain.Review, error)
	// UpdateReviewContent persists only rating and comment of the still
	// kept review under id and returns the stored row, or (a wrapped)
	// domain.ErrReviewNotFound when it is missing or discarded.
	// Column-scoped so discarded_at is never written.
	UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error)
	// DiscardReview soft-deletes the review (stamps discarded_at, never a
	// hard DELETE), or returns (a wrapped) domain.ErrReviewNotFound when
	// it is missing or already discarded.
	DiscardReview(ctx context.Context, id int64) error
}

// Reviews implements the review use cases: the public feed and detail,
// and the author-scoped create/edit/delete.
type Reviews struct {
	repo ReviewRepository
}

func NewReviews(repo ReviewRepository) *Reviews { return &Reviews{repo: repo} }

// List returns the public review feed, paginated with the same fallback
// rules as Shops.List: page < 1 becomes 1, perPage < 1 becomes 20, and
// perPage is capped at 100.
func (s *Reviews) List(ctx context.Context, page, perPage int) ([]domain.ReviewDetail, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultReviewsPerPage
	}
	if perPage > maxReviewsPerPage {
		perPage = maxReviewsPerPage
	}
	// Far-out pages yield an empty list; clamping page before the
	// multiplication keeps the product inside int64, and clamping the
	// offset keeps it in int32 without changing that outcome (same
	// reasoning as Shops.List).
	if page > math.MaxInt32 {
		page = math.MaxInt32
	}
	offset := min(int64(page-1)*int64(perPage), math.MaxInt32)
	reviews, err := s.repo.ListReviews(ctx, int32(perPage), int32(offset))
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	return reviews, nil
}

// Get returns one review with author, burger, and stats. Missing and
// discarded reviews both yield domain.ErrReviewNotFound.
func (s *Reviews) Get(ctx context.Context, id int64) (domain.ReviewDetail, error) {
	detail, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("get review: %w", err)
	}
	return detail, nil
}

// Create posts a review by viewer for a burger of a shop, in the
// contract's check order: unknown shop (404), then the domain reviewable
// rule (403), then unknown or unlinked burger (404), then content
// validation (422). The response detail is composed from the viewer and
// the burger already fetched for the existence check — no re-fetch.
func (s *Reviews) Create(ctx context.Context, viewer domain.User, shopID, burgerID int64, rating int, comment string) (domain.ReviewDetail, error) {
	shop, err := s.repo.GetShop(ctx, shopID)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	if !shop.CanBeReviewedBy(viewer) {
		return domain.ReviewDetail{}, domain.ErrForbidden
	}
	burger, err := s.repo.GetShopBurger(ctx, shopID, burgerID)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	review, err := domain.NewReview(rating, comment, viewer.ID, burgerID)
	if err != nil {
		return domain.ReviewDetail{}, err
	}
	created, err := s.repo.CreateReview(ctx, review)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	s.burgerStatsChanged(ctx, burgerID)
	return domain.ReviewDetail{
		Review: created,
		User:   &domain.UserRef{ID: viewer.ID, Username: viewer.Username},
		Burger: &burger,
	}, nil
}

// Update edits a review's rating and comment: load (404 for missing and
// discarded alike), the domain ownership rule (403 — issue #14 AC3, an
// admin gets no pass), content validation (422), then the column-scoped
// write. The stored row is merged into the loaded detail so the response
// carries author, burger, and stats without a re-fetch.
func (s *Reviews) Update(ctx context.Context, viewer domain.User, id int64, rating int, comment string) (domain.ReviewDetail, error) {
	detail, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	if !detail.CanBeModifiedBy(viewer) {
		return domain.ReviewDetail{}, domain.ErrForbidden
	}
	if err := domain.ValidateReviewContent(rating, comment); err != nil {
		return domain.ReviewDetail{}, err
	}
	updated, err := s.repo.UpdateReviewContent(ctx, id, rating, comment)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	detail.Review = updated
	s.burgerStatsChanged(ctx, updated.BurgerID)
	return detail, nil
}

// Delete soft-deletes a review: load (404), the domain ownership rule
// (403, author-only like Update), then the column-scoped discard — never
// a hard DELETE.
func (s *Reviews) Delete(ctx context.Context, viewer domain.User, id int64) error {
	detail, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	if !detail.CanBeModifiedBy(viewer) {
		return domain.ErrForbidden
	}
	if err := s.repo.DiscardReview(ctx, id); err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	s.burgerStatsChanged(ctx, detail.BurgerID)
	return nil
}

// burgerStatsChanged is the explicit S7 hook point, called after every
// successful review create, update, and discard for the affected burger.
func (s *Reviews) burgerStatsChanged(ctx context.Context, burgerID int64) {
	// TODO(S7): trigger burger_stats recalculation for burgerID
}
