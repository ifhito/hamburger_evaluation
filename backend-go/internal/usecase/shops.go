package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// Pagination bounds for shop listing (Rails parity).
const (
	defaultShopsPerPage = 20
	maxShopsPerPage     = 100
)

// ShopRepository is the consumer-side persistence contract for shop reads.
// Implementations translate the visibility descriptor into SQL parameters
// (the rule itself lives in domain.ShopVisibility) and return
// (a wrapped) domain.ErrShopNotFound when no shop matches an id.
type ShopRepository interface {
	// ListShops returns visible shops matching keyword (literal substring
	// of the name, case-insensitive; empty matches all), ordered by name
	// then id.
	ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error)
	// GetShopWithCreator returns the shop and its creator, with Reviews
	// left empty.
	GetShopWithCreator(ctx context.Context, id int64) (domain.ShopDetail, error)
	// ListShopReviews returns the non-discarded reviews of the shop's
	// burgers, newest first (created_at desc, id desc).
	ListShopReviews(ctx context.Context, shopID int64) ([]domain.ShopReview, error)
}

// Shops implements the shop read use cases (list and detail).
type Shops struct {
	repo ShopRepository
}

func NewShops(repo ShopRepository) *Shops { return &Shops{repo: repo} }

// List returns the shops visible to viewer (nil = anonymous) matching
// keyword, paginated. Out-of-range page/perPage fall back to defaults
// instead of erroring: page < 1 becomes 1, perPage < 1 becomes 20, and
// perPage is capped at 100.
func (s *Shops) List(ctx context.Context, viewer *domain.User, keyword string, page, perPage int) ([]domain.Shop, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultShopsPerPage
	}
	if perPage > maxShopsPerPage {
		perPage = maxShopsPerPage
	}
	// Far-out pages yield an empty list; clamping page before the
	// multiplication keeps the product (at most (2^31-1)*100) inside int64,
	// and clamping the offset keeps it in int32 without changing that
	// outcome.
	if page > math.MaxInt32 {
		page = math.MaxInt32
	}
	offset := min(int64(page-1)*int64(perPage), math.MaxInt32)
	shops, err := s.repo.ListShops(ctx, domain.ShopVisibilityFor(viewer), keyword, int32(perPage), int32(offset))
	if err != nil {
		return nil, fmt.Errorf("list shops: %w", err)
	}
	return shops, nil
}

// Get returns the shop detail (creator and reviews included) when viewer
// may see it. A missing shop and a hidden shop both yield
// domain.ErrShopNotFound so existence is not leaked.
func (s *Shops) Get(ctx context.Context, viewer *domain.User, id int64) (domain.ShopDetail, error) {
	detail, err := s.repo.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("get shop: %w", err)
	}
	if !domain.ShopVisibilityFor(viewer).CanView(detail.Shop) {
		return domain.ShopDetail{}, domain.ErrShopNotFound
	}
	reviews, err := s.repo.ListShopReviews(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("list shop reviews: %w", err)
	}
	detail.Reviews = reviews
	return detail, nil
}
