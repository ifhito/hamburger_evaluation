package domain

import (
	"strings"
	"time"
)

// ShopStatus is the API-facing shop status string. Storage encodes it as
// a smallint; that mapping lives at the repository/domain boundary.
type ShopStatus string

const (
	ShopStatusPending  ShopStatus = "pending"
	ShopStatusActive   ShopStatus = "active"
	ShopStatusRejected ShopStatus = "rejected"
)

// Shop is the domain representation of a shop.
type Shop struct {
	ID             int64
	Name           string
	Status         ShopStatus
	ModerationNote *string
	CreatorID      *int64
}

// ValidateShopName enforces the Rails presence validation on the shop
// name: a blank or whitespace-only name yields the exact Rails full
// message inside a *ValidationError.
func ValidateShopName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &ValidationError{Messages: []string{"Name can't be blank"}}
	}
	return nil
}

// NewShopSubmission builds a user-submitted shop: the name is validated,
// the status starts pending (Rails ShopStatus.initial), there is no
// moderation note yet, and the submitting user is recorded as creator.
func NewShopSubmission(name string, creatorID int64) (Shop, error) {
	if err := ValidateShopName(name); err != nil {
		return Shop{}, err
	}
	return Shop{Name: name, Status: ShopStatusPending, CreatorID: &creatorID}, nil
}

// Approve is the moderation transition to active. Like Rails ShopStatus,
// it is an unconditional value transition from any current status —
// re-approving a rejected shop is allowed — and it clears the moderation
// note, which only ever explains a rejection.
func (s Shop) Approve() Shop {
	s.Status = ShopStatusActive
	s.ModerationNote = nil
	return s
}

// Reject is the moderation transition to rejected, from any current
// status. The optional note replaces the previous one (nil clears it).
func (s Shop) Reject(note *string) Shop {
	s.Status = ShopStatusRejected
	s.ModerationNote = note
	return s
}

// CanBeReviewedBy is the single home of the reviewable rule: whether
// viewer (always authenticated — posting requires a login) may post a
// review for a burger of this shop. Rejected shops are never reviewable
// (even by their creator or an admin, who may still view them via
// ShopVisibility.CanView), active shops are reviewable by anyone
// authenticated, and pending shops only by their creator or an admin.
func (s Shop) CanBeReviewedBy(viewer User) bool {
	switch s.Status {
	case ShopStatusActive:
		return true
	case ShopStatusPending:
		return viewer.Admin || (s.CreatorID != nil && *s.CreatorID == viewer.ID)
	default:
		return false
	}
}

// ShopVisibility is the filter descriptor derived from a viewer. A shop
// is visible iff ViewAll is set, or the shop is active, or its creator is
// ViewerID. This type is the single home of the shop visibility rule:
// CanView applies it in-process and repositories only translate the
// descriptor into SQL parameters.
type ShopVisibility struct {
	// ViewAll grants visibility of every shop regardless of status (admin).
	ViewAll bool
	// ViewerID, when non-nil, additionally grants visibility of shops
	// created by this user, whatever their status.
	ViewerID *int64
}

// ShopVisibilityFor derives the visibility descriptor for viewer; nil
// means anonymous (active shops only).
func ShopVisibilityFor(viewer *User) ShopVisibility {
	if viewer == nil {
		return ShopVisibility{}
	}
	if viewer.Admin {
		return ShopVisibility{ViewAll: true}
	}
	id := viewer.ID
	return ShopVisibility{ViewerID: &id}
}

// CanView reports whether a shop is visible under this descriptor.
func (v ShopVisibility) CanView(shop Shop) bool {
	if v.ViewAll || shop.Status == ShopStatusActive {
		return true
	}
	return v.ViewerID != nil && shop.CreatorID != nil && *shop.CreatorID == *v.ViewerID
}

// UserRef is the {id, username} projection embedded in shop detail and
// review payloads.
type UserRef struct {
	ID       int64
	Username string
}

// ShopDetail is a shop with its creator and non-discarded reviews.
type ShopDetail struct {
	Shop
	Creator *UserRef // nil when the shop has no creator
	Reviews []ShopReview
}

// ShopReview is one review shown on a shop detail, with its author and
// the reviewed burger including review-derived statistics.
type ShopReview struct {
	ID        int64
	Rating    int
	Comment   *string
	CreatedAt time.Time
	User      *UserRef
	Burger    *ShopReviewBurger
}

// ShopReviewBurger is the reviewed burger with its statistics; the stats
// are zero when none have been calculated yet.
type ShopReviewBurger struct {
	ID            int64
	Name          string
	AverageRating float64
	ReviewCount   int64
	WeightedScore float64
	Confidence    float64
}
