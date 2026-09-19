package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// ReviewRepository is the consumer-side persistence contract for reviews.
// Implementations keep the SQL details (EXISTS active-shop filter, the
// smallint status encoding, soft-delete predicates) to themselves and
// return wrapped domain sentinels (ErrReviewNotFound, ErrShopNotFound,
// ErrBurgerNotFound) when no row matches.
type ReviewRepository interface {
	// ListReviews returns the non-discarded reviews whose burger is
	// served by at least one active shop, narrowed by filter, with
	// author, burger, and stats joined (no N+1), newest first
	// (created_at desc, id desc).
	ListReviews(ctx context.Context, filter ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error)
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
	// it with its generated id and created_at. The write also recalculates
	// the burger's burger_stats in the same transaction (issue #15, S7).
	CreateReview(ctx context.Context, review domain.Review) (domain.Review, error)
	// CreateReviewForNamedBurger persists a new (already validated) review
	// against the shop's burger with the given exact name, creating the
	// burger and its shops_burgers link when the shop has none by that name
	// (Rails find_or_create_burger, S6 P3-1). The review's BurgerID input
	// is ignored and set to the resolved burger. Find-or-create, review
	// insert, and burger_stats recalculation happen in ONE transaction, so
	// a failed insert leaves no orphan burger or link. The returned burger
	// carries the stats as stored before the insert — exactly what
	// GetShopBurger yields on the burger_id path; a brand-new burger has
	// zero stats.
	CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error)
	// UpdateReviewContent persists only rating and comment of the still
	// kept review under id and returns the stored row, or (a wrapped)
	// domain.ErrReviewNotFound when it is missing or discarded.
	// Column-scoped so discarded_at is never written. The write also
	// recalculates the burger's burger_stats in the same transaction.
	UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error)
	// UpdateReviewPhotoKey persists only photo_key of the still kept
	// review under id and returns the stored row, or (a wrapped)
	// domain.ErrReviewNotFound when it is missing or discarded (S10).
	// photo_key plays no role in burger_stats, so no recalculation.
	UpdateReviewPhotoKey(ctx context.Context, id int64, photoKey *string) (domain.Review, error)
	// UpdateReviewContentAndPhotoKey persists rating, comment, AND
	// photo_key of the still kept review under id atomically — the two
	// column-scoped writes plus the burger_stats recalculation share ONE
	// transaction, so a photo-carrying edit can never commit the content
	// without the key (S10 review fix). Returns the stored row, or (a
	// wrapped) domain.ErrReviewNotFound when the review is missing or
	// discarded (nothing is committed then).
	UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error)
	// DiscardReview soft-deletes the review (stamps discarded_at, never a
	// hard DELETE), or returns (a wrapped) domain.ErrReviewNotFound when
	// it is missing or already discarded. The write also recalculates the
	// burger's burger_stats in the same transaction.
	DiscardReview(ctx context.Context, id int64) error
}

// ReviewListFilter carries the optional GET /reviews query filters,
// mirroring Rails ReviewQuery: each present filter narrows the feed, all
// present filters combine with AND. A nil Rating/ShopID and an empty
// Keyword mean "absent" (Rails params[:x].present?), so a present zero or
// negative id/rating still filters (to an empty page) exactly like Rails.
type ReviewListFilter struct {
	// Rating is an exact-match rating filter (Rails by_rating).
	Rating *int
	// Keyword is a literal case-insensitive substring match on comment
	// (Rails keyword_search, comment ILIKE %escaped%).
	Keyword string
	// ShopID keeps only reviews whose burger is linked to that shop via
	// shops_burgers (Rails' shops_and_burgers join).
	ShopID *int64
}

// Reviews implements the review use cases: the public feed and detail,
// and the author-scoped create/edit/delete, with an optional photo per
// review stored via photos (S10).
type Reviews struct {
	repo   ReviewRepository
	photos PhotoStorage
}

// NewReviews wires the review use cases. photos must be non-nil (disk or
// S3 in production, a fake in tests): every request path may dereference
// it (photoURL, deletePhotoBestEffort), so a nil storage fails loudly
// here instead of panicking mid-request.
func NewReviews(repo ReviewRepository, photos PhotoStorage) *Reviews {
	if photos == nil {
		panic("usecase.NewReviews: nil PhotoStorage")
	}
	return &Reviews{repo: repo, photos: photos}
}

// List returns the public review feed narrowed by filter, paginated with
// the same fallback rules as Shops.List, per clampPage.
func (s *Reviews) List(ctx context.Context, filter ReviewListFilter, page, perPage int) ([]domain.ReviewDetail, error) {
	limit, offset := clampPage(page, perPage)
	reviews, err := s.repo.ListReviews(ctx, filter, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	for i := range reviews {
		reviews[i].PhotoURL = s.photoURL(reviews[i].PhotoKey)
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
	detail.PhotoURL = s.photoURL(detail.PhotoKey)
	return detail, nil
}

// Create posts a review by viewer for a burger of a shop, in the
// contract's check order: unknown shop (404), then the domain reviewable
// rule (403), then the burger resolution, then content validation (422).
// A positive burgerID takes precedence and must name a burger linked to
// the shop (404 otherwise); else a non-blank burgerName find-or-creates
// the shop's burger by exact, untrimmed name (Rails parity, S6 P3-1)
// inside the same transaction as the insert; neither is a validation
// failure (422) — never a silent default. The response detail is composed
// from the viewer and the burger resolved for the existence check — no
// re-fetch. A non-nil upload (already validated/normalized by the
// handler, S10) is stored under a fresh random key before the insert; a
// failed insert then best-effort deletes the just-uploaded blob so no
// orphan file outlives the request.
func (s *Reviews) Create(ctx context.Context, viewer domain.User, shopID, burgerID int64, burgerName string, rating int, comment string, upload *photo.Processed) (domain.ReviewDetail, error) {
	shop, err := s.repo.GetShop(ctx, shopID)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	if !shop.CanBeReviewedBy(viewer) {
		return domain.ReviewDetail{}, domain.ErrForbidden
	}
	var burger domain.ShopReviewBurger
	if burgerID > 0 {
		if burger, err = s.repo.GetShopBurger(ctx, shopID, burgerID); err != nil {
			return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
		}
	} else if err := domain.ValidateBurgerName(burgerName); err != nil {
		return domain.ReviewDetail{}, err
	}
	// BurgerID 0: the repository resolves it inside the transaction.
	review, err := domain.NewReview(rating, comment, viewer.ID, burgerID)
	if err != nil {
		return domain.ReviewDetail{}, err
	}
	if review.PhotoKey, err = s.putPhoto(ctx, upload); err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	var created domain.Review
	if burgerID > 0 {
		created, err = s.repo.CreateReview(ctx, review)
	} else {
		created, burger, err = s.repo.CreateReviewForNamedBurger(ctx, shopID, burgerName, review)
	}
	if err != nil {
		s.deletePhotoBestEffort(ctx, review.PhotoKey)
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	return domain.ReviewDetail{
		Review:   created,
		User:     &domain.UserRef{ID: viewer.ID, Username: viewer.Username},
		Burger:   &burger,
		PhotoURL: s.photoURL(created.PhotoKey),
	}, nil
}

// Update edits a review's rating and comment: load (404 for missing and
// discarded alike), the domain ownership rule (403 — issue #14 AC3, an
// admin gets no pass), content validation (422), then the column-scoped
// write. The stored row is merged into the loaded detail so the response
// carries author, burger, and stats without a re-fetch. A non-nil upload
// (S10) replaces the photo: the new blob is stored first, then content
// and photo_key are switched in ONE repository transaction (so a failure
// can never commit the content without the key), and only after that DB
// success is the old blob best-effort deleted. A nil upload takes the
// content-only write and leaves photo_key untouched (there is no
// photo-removal path).
func (s *Reviews) Update(ctx context.Context, viewer domain.User, id int64, rating int, comment string, upload *photo.Processed) (domain.ReviewDetail, error) {
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
	newKey, err := s.putPhoto(ctx, upload)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	var updated domain.Review
	if newKey != nil {
		if updated, err = s.repo.UpdateReviewContentAndPhotoKey(ctx, id, rating, comment, newKey); err != nil {
			s.deletePhotoBestEffort(ctx, newKey)
			return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
		}
		// The old blob is unreferenced only now that the DB points at the
		// new key; losing it is a leaked file, not a broken review.
		s.deletePhotoBestEffort(ctx, detail.PhotoKey)
	} else if updated, err = s.repo.UpdateReviewContent(ctx, id, rating, comment); err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	detail.Review = updated
	detail.PhotoURL = s.photoURL(updated.PhotoKey)
	return detail, nil
}

// Delete soft-deletes a review: load (404), the domain ownership rule
// (403, author-only like Update), then the column-scoped discard — never
// a hard DELETE. The photo blob, if any, is best-effort deleted after the
// discard succeeded (S10).
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
	s.deletePhotoBestEffort(ctx, detail.PhotoKey)
	return nil
}

// putPhoto stores the processed upload under a fresh random key
// ("reviews/<32 hex chars><ext>", crypto/rand — collisions are
// negligible) and returns that key; a nil upload yields a nil key and no
// storage call.
func (s *Reviews) putPhoto(ctx context.Context, upload *photo.Processed) (*string, error) {
	if upload == nil {
		return nil, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("put photo: random key: %w", err)
	}
	key := "reviews/" + hex.EncodeToString(raw[:]) + upload.Ext
	if err := s.photos.Put(ctx, key, upload.ContentType, bytes.NewReader(upload.Data)); err != nil {
		return nil, fmt.Errorf("put photo %q: %w", key, err)
	}
	return &key, nil
}

// deletePhotoBestEffort removes the blob under key, if any. This is the
// documented best-effort exception to fail-loud: the DB is already the
// source of truth by the time it runs, so a storage failure here means a
// leaked (or already-gone) blob, never a broken review — it is logged
// with the key and the request still succeeds. The delete runs detached
// from the request's cancellation (WithoutCancel) under its own short
// timeout: a client that hangs up must not turn every delete into a
// guaranteed orphan (S3 mode), while the timeout keeps the call bounded.
func (s *Reviews) deletePhotoBestEffort(ctx context.Context, key *string) {
	if key == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.photos.Delete(ctx, *key); err != nil {
		slog.Warn("best-effort review photo delete failed", "key", *key, "error", err)
	}
}

// photoURL maps a stored photo key onto its public URL (nil in, nil out).
func (s *Reviews) photoURL(key *string) *string {
	if key == nil {
		return nil
	}
	url := s.photos.URL(*key)
	return &url
}
