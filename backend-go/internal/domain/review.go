package domain

import (
	"strings"
	"time"
)

// Review is the domain representation of a burger review row.
type Review struct {
	ID        int64
	Rating    int
	Comment   *string
	AuthorID  int64
	BurgerID  int64
	CreatedAt time.Time
}

// ValidateReviewContent enforces the Rails validations on the writable
// review attributes: the rating must be an integer in 1..5 and the comment
// must be present. Failures yield the exact Rails full messages inside a
// *ValidationError, rating message first.
func ValidateReviewContent(rating int, comment string) error {
	var messages []string
	if rating < 1 || rating > 5 {
		messages = append(messages, "Rating must be in 1..5")
	}
	if strings.TrimSpace(comment) == "" {
		messages = append(messages, "Comment can't be blank")
	}
	if len(messages) > 0 {
		return &ValidationError{Messages: messages}
	}
	return nil
}

// NewReview builds a validated new review by author for burger. The
// comment is stored as given (only its presence is validated), matching
// Rails which never trims user text.
func NewReview(rating int, comment string, authorID, burgerID int64) (Review, error) {
	if err := ValidateReviewContent(rating, comment); err != nil {
		return Review{}, err
	}
	c := comment
	return Review{Rating: rating, Comment: &c, AuthorID: authorID, BurgerID: burgerID}, nil
}

// CanBeModifiedBy is the single home of the review ownership rule: only
// the author may edit or delete a review. Deliberately no admin pass
// (issue #14 AC3) — moderation powers cover shops, not other users'
// reviews.
func (r Review) CanBeModifiedBy(viewer User) bool {
	return r.AuthorID == viewer.ID
}

// ReviewDetail is a review with its author and the reviewed burger
// including review-derived statistics — the payload of the review
// endpoints. The stats are zero when none have been calculated yet.
type ReviewDetail struct {
	Review
	User   *UserRef
	Burger *ShopReviewBurger
}
