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

// ShopRepository is the consumer-side persistence contract for shops.
// Implementations translate the visibility descriptor into SQL parameters
// (the rule itself lives in domain.ShopVisibility), keep the smallint
// status encoding to themselves, and return (a wrapped)
// domain.ErrShopNotFound when no shop matches an id.
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
	// CreateShop persists a new shop and returns it with its generated id.
	CreateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error)
	// ListShopsForModeration returns every shop with its creator (Reviews
	// left empty), newest first (created_at desc, id desc), optionally
	// filtered to one status (nil = all).
	ListShopsForModeration(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error)
	// UpdateShopName persists only the shop's name under id and returns
	// the stored row. Column-scoped so a concurrent status change is
	// never reverted by a stale snapshot.
	UpdateShopName(ctx context.Context, id int64, name string) (domain.Shop, error)
	// UpdateShopStatus persists only the shop's status and moderation
	// note under id and returns the stored row. Column-scoped so a
	// concurrent rename is never reverted by a stale snapshot.
	UpdateShopStatus(ctx context.Context, id int64, status domain.ShopStatus, note *string) (domain.Shop, error)
}

// Shops implements the shop use cases: public list and detail, user
// submission, and admin moderation.
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

// Create submits a new shop on behalf of viewer: it starts pending (a
// moderator activates it later) with viewer recorded as creator. A blank
// name surfaces the domain *ValidationError unchanged.
func (s *Shops) Create(ctx context.Context, viewer domain.User, name string) (domain.ShopDetail, error) {
	shop, err := domain.NewShopSubmission(name, viewer.ID)
	if err != nil {
		return domain.ShopDetail{}, err
	}
	created, err := s.repo.CreateShop(ctx, shop)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("create shop: %w", err)
	}
	// The creator is the viewer itself, so the response ref is composed
	// here instead of re-fetching the row with a join.
	return domain.ShopDetail{
		Shop:    created,
		Creator: &domain.UserRef{ID: viewer.ID, Username: viewer.Username},
	}, nil
}

// AdminList returns every shop for the moderation screen, newest first,
// optionally filtered by the status string of the API. An unknown status
// matches nothing (like Rails where(status: unknown)) instead of erroring.
// Non-admin viewers get domain.ErrForbidden.
func (s *Shops) AdminList(ctx context.Context, viewer domain.User, status string) ([]domain.ShopDetail, error) {
	if !viewer.Admin {
		return nil, domain.ErrForbidden
	}
	var filter *domain.ShopStatus
	if status != "" {
		switch st := domain.ShopStatus(status); st {
		case domain.ShopStatusPending, domain.ShopStatusActive, domain.ShopStatusRejected:
			filter = &st
		default:
			return []domain.ShopDetail{}, nil
		}
	}
	shops, err := s.repo.ListShopsForModeration(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("admin list shops: %w", err)
	}
	return shops, nil
}

// AdminUpdateName renames a shop (the only moderation edit, Rails
// parity). Non-admin viewers get domain.ErrForbidden before any lookup so
// they cannot probe which ids exist; a blank name is a *ValidationError.
func (s *Shops) AdminUpdateName(ctx context.Context, viewer domain.User, id int64, name string) (domain.ShopDetail, error) {
	if !viewer.Admin {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	if err := domain.ValidateShopName(name); err != nil {
		return domain.ShopDetail{}, err
	}
	// The fetch supplies the creator for the response (and a 404 for
	// unknown ids); the write itself touches only the name column so it
	// cannot revert a concurrent status change.
	detail, err := s.repo.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("admin update shop name: %w", err)
	}
	updated, err := s.repo.UpdateShopName(ctx, id, name)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("admin update shop name: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}

// Approve activates a shop and clears its moderation note (the domain
// transition), making it publicly visible. Non-admin viewers get
// domain.ErrForbidden before any lookup.
func (s *Shops) Approve(ctx context.Context, viewer domain.User, id int64) (domain.ShopDetail, error) {
	if !viewer.Admin {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	return s.moderate(ctx, id, domain.Shop.Approve)
}

// Reject rejects a shop with an optional moderation note, hiding it from
// the public list. Non-admin viewers get domain.ErrForbidden before any
// lookup.
func (s *Shops) Reject(ctx context.Context, viewer domain.User, id int64, note *string) (domain.ShopDetail, error) {
	if !viewer.Admin {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	return s.moderate(ctx, id, func(shop domain.Shop) domain.Shop {
		return shop.Reject(note)
	})
}

// moderate is the shared flow of the status transitions: load the shop
// with its creator (for the response and the 404), apply the domain
// transition, persist only its status and moderation note, and return the
// detail carrying the stored row. The column-scoped write cannot revert a
// concurrent rename from the stale snapshot.
func (s *Shops) moderate(ctx context.Context, id int64, transition func(domain.Shop) domain.Shop) (domain.ShopDetail, error) {
	detail, err := s.repo.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("moderate shop: %w", err)
	}
	next := transition(detail.Shop)
	updated, err := s.repo.UpdateShopStatus(ctx, id, next.Status, next.ModerationNote)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("moderate shop: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}
