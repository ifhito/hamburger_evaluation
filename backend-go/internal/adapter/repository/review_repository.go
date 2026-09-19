package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ReviewRepository implements usecase.ReviewRepository over sqlc-generated
// queries. The soft-delete predicate (discarded_at IS NULL) and the
// active-shop EXISTS filter live in the SQL; the authorization rules
// themselves live in the domain package. Every write (create, edit,
// discard) also recalculates the affected burger's burger_stats row inside
// the same transaction, serialized per burger via LockBurgerForStats.
type ReviewRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewReviewRepository wraps db (normally the shared pgx pool).
func NewReviewRepository(db beginnerDBTX) *ReviewRepository {
	return &ReviewRepository{db: db, q: sqlcgen.New(db)}
}

var _ usecase.ReviewRepository = (*ReviewRepository)(nil)

// ListReviews returns the public review feed: non-discarded reviews whose
// burger is linked to at least one active shop, narrowed by filter (Rails
// ReviewQuery parity), with author, burger, and stats in a single query
// (no N+1), newest first. The keyword goes through likeEscaper into an
// ILIKE parameter, never concatenated into SQL; absent filters stay NULL.
func (r *ReviewRepository) ListReviews(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
	params := sqlcgen.ListPublicReviewsParams{
		PageLimit:  limit,
		PageOffset: offset,
	}
	if filter.Rating != nil {
		params.FilterRating = pgtype.Int8{Int64: int64(*filter.Rating), Valid: true}
	}
	if filter.Keyword != "" {
		params.CommentPattern = pgtype.Text{String: "%" + likeEscaper.Replace(filter.Keyword) + "%", Valid: true}
	}
	if filter.ShopID != nil {
		params.FilterShopID = pgtype.Int8{Int64: *filter.ShopID, Valid: true}
	}
	rows, err := r.q.ListPublicReviews(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	reviews := make([]domain.ReviewDetail, 0, len(rows))
	for _, row := range rows {
		reviews = append(reviews, toReviewDetail(
			row.ID, row.Rating, row.Comment, row.PhotoKey, row.CreatedAt,
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
		row.ID, row.Rating, row.Comment, row.PhotoKey, row.CreatedAt,
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
// its generated id and created_at. The insert and the burger_stats
// recalculation happen in one transaction so the stats can never lag or
// outlive the review.
func (r *ReviewRepository) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	var row sqlcgen.Review
	err := withTx(ctx, r.db, "create review", func(q *sqlcgen.Queries) error {
		var err error
		row, err = insertReviewAndRecalc(ctx, q, review, "create review")
		return err
	})
	if err != nil {
		return domain.Review{}, err
	}
	return toDomainReview(row), nil
}

// CreateReviewForNamedBurger inserts the (already validated) review against
// the shop's burger with the exact name burgerName, creating the burger and
// its shops_burgers link when the shop has none by that name (Rails
// find_or_create_burger, S6 P3-1). Find-or-create, review insert, and
// burger_stats recalculation share ONE transaction, so a failure at any
// step commits no orphan burger or link. There is deliberately no unique
// index on (shop, name): two concurrent creators of the same new name can
// both insert a burger — the same race Rails' find_or_create_burger has;
// parity, not a bug. The returned burger carries the pre-insert stats,
// exactly like GetShopBurger on the burger_id path (zeros for a brand-new
// burger).
func (r *ReviewRepository) CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
	var row sqlcgen.Review
	var burger domain.ShopReviewBurger
	err := withTx(ctx, r.db, "create review for named burger", func(q *sqlcgen.Queries) error {
		found, err := q.GetShopBurgerByNameWithStats(ctx, sqlcgen.GetShopBurgerByNameWithStatsParams{
			ShopID: shopID,
			Name:   burgerName,
		})
		switch {
		case err == nil:
			burger = domain.ShopReviewBurger{
				ID:   found.ID,
				Name: found.Name,
				// The stats row may not exist yet; zero values then.
				AverageRating: found.AverageRating.Float64,
				ReviewCount:   found.ReviewCount.Int64,
				WeightedScore: found.WeightedScore.Float64,
				Confidence:    found.Confidence.Float64,
			}
		case errors.Is(err, pgx.ErrNoRows):
			created, err := q.CreateBurger(ctx, burgerName)
			if err != nil {
				return fmt.Errorf("create review for named burger: create burger: %w", err)
			}
			if err := q.CreateShopBurger(ctx, sqlcgen.CreateShopBurgerParams{ShopID: shopID, BurgerID: created.ID}); err != nil {
				return fmt.Errorf("create review for named burger: link burger: %w", err)
			}
			burger = domain.ShopReviewBurger{ID: created.ID, Name: created.Name}
		default:
			return fmt.Errorf("create review for named burger: find burger: %w", err)
		}
		review.BurgerID = burger.ID
		row, err = insertReviewAndRecalc(ctx, q, review, "create review for named burger")
		return err
	})
	if err != nil {
		return domain.Review{}, domain.ShopReviewBurger{}, err
	}
	return toDomainReview(row), burger, nil
}

// UpdateReviewContent persists only rating and comment of the still kept
// review under id and returns the stored row, or domain.ErrReviewNotFound
// when it is missing or discarded (the transaction is rolled back, so the
// stats stay untouched). Column-scoped: discarded_at is never written, so
// an edit can neither resurrect nor race a soft delete. The update and the
// burger_stats recalculation happen in one transaction.
func (r *ReviewRepository) UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error) {
	var row sqlcgen.Review
	err := withTx(ctx, r.db, "update review content", func(q *sqlcgen.Queries) error {
		var err error
		row, err = q.UpdateReviewContent(ctx, sqlcgen.UpdateReviewContentParams{
			ID:      id,
			Rating:  int16(rating),
			Comment: pgtype.Text{String: comment, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("update review content: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("update review content: %w", err)
		}
		if err := recalculateBurgerStats(ctx, q, row.BurgerID); err != nil {
			return fmt.Errorf("update review content: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Review{}, err
	}
	return toDomainReview(row), nil
}

// UpdateReviewPhotoKey persists only photo_key of the still kept review
// under id and returns the stored row, or domain.ErrReviewNotFound when it
// is missing or discarded. photo_key plays no role in burger_stats, so
// unlike the other writes this one needs no transaction and no
// recalculation.
func (r *ReviewRepository) UpdateReviewPhotoKey(ctx context.Context, id int64, photoKey *string) (domain.Review, error) {
	row, err := r.q.UpdateReviewPhotoKey(ctx, sqlcgen.UpdateReviewPhotoKeyParams{
		ID:       id,
		PhotoKey: textOrNull(photoKey),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Review{}, fmt.Errorf("update review photo key: %w", domain.ErrReviewNotFound)
		}
		return domain.Review{}, fmt.Errorf("update review photo key: %w", err)
	}
	return toDomainReview(row), nil
}

// DiscardReview soft-deletes the review (stamps discarded_at, never a
// hard DELETE). Missing and already-discarded reviews match no row and
// yield domain.ErrReviewNotFound (the transaction is rolled back, so the
// stats stay untouched). The discard and the burger_stats recalculation
// happen in one transaction.
func (r *ReviewRepository) DiscardReview(ctx context.Context, id int64) error {
	return withTx(ctx, r.db, "discard review", func(q *sqlcgen.Queries) error {
		burgerID, err := q.DiscardReview(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("discard review: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("discard review: %w", err)
		}
		if err := recalculateBurgerStats(ctx, q, burgerID); err != nil {
			return fmt.Errorf("discard review: %w", err)
		}
		return nil
	})
}

// insertReviewAndRecalc is the shared tail of both create paths: lock the
// burger, insert the review, recalculate its burger_stats. It runs inside
// the caller's transaction — q must be tx-scoped, and the helper never
// opens a transaction of its own. The lock comes BEFORE the insert: the
// insert's FK check takes a KEY SHARE lock on the burgers row, and
// upgrading it to FOR UPDATE afterwards could deadlock two concurrent
// creators. recalculateBurgerStats' own lock is then a free
// re-acquisition (row locks are transaction-owned in PostgreSQL). op
// prefixes the error messages, preserving each call site's wording.
func insertReviewAndRecalc(ctx context.Context, q *sqlcgen.Queries, review domain.Review, op string) (sqlcgen.Review, error) {
	if _, err := q.LockBurgerForStats(ctx, review.BurgerID); err != nil {
		return sqlcgen.Review{}, fmt.Errorf("%s: lock burger: %w", op, err)
	}
	row, err := q.CreateReview(ctx, sqlcgen.CreateReviewParams{
		Rating:   int16(review.Rating),
		Comment:  textOrNull(review.Comment),
		UserID:   review.AuthorID,
		BurgerID: review.BurgerID,
		PhotoKey: textOrNull(review.PhotoKey),
	})
	if err != nil {
		return sqlcgen.Review{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := recalculateBurgerStats(ctx, q, review.BurgerID); err != nil {
		return sqlcgen.Review{}, fmt.Errorf("%s: %w", op, err)
	}
	return row, nil
}

// recalculateBurgerStats recomputes and upserts the burger's stats row
// from its kept reviews via the domain calculator, inside the caller's
// transaction: q must be tx-scoped. The helper itself takes the per-burger
// FOR UPDATE lock via LockBurgerForStats first (see that query for the
// lost-update rationale); re-acquiring a lock the transaction already
// holds is a no-op. Callers recalculating MULTIPLE burgers in one
// transaction (the S8 user-discard flow, UserRepository.DiscardUser) must
// invoke it per burger in ascending burger_id order so overlapping burger
// sets cannot deadlock. Zero kept reviews still upsert the zero row
// (Rails BurgerScore.empty).
func recalculateBurgerStats(ctx context.Context, q *sqlcgen.Queries, burgerID int64) error {
	if _, err := q.LockBurgerForStats(ctx, burgerID); err != nil {
		return fmt.Errorf("recalculate burger stats: lock burger: %w", err)
	}
	rows, err := q.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return fmt.Errorf("recalculate burger stats: list facts: %w", err)
	}
	// Reviewer-trust histories for the distinct fact authors: each one's
	// kept ratings across all burgers, grouped by user.
	historyByUser := make(map[int64][]float64, len(rows))
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if _, seen := historyByUser[row.UserID]; !seen {
			historyByUser[row.UserID] = nil
			userIDs = append(userIDs, row.UserID)
		}
	}
	if len(userIDs) > 0 {
		ratings, err := q.ListReviewerRatings(ctx, userIDs)
		if err != nil {
			return fmt.Errorf("recalculate burger stats: list reviewer ratings: %w", err)
		}
		for _, rating := range ratings {
			historyByUser[rating.UserID] = append(historyByUser[rating.UserID], float64(rating.Rating))
		}
	}
	facts := make([]domain.ReviewFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(row.Rating),
			CreatedAt:       row.CreatedAt.Time,
			ReviewerHistory: domain.ReviewerHistory{Ratings: historyByUser[row.UserID]},
		})
	}
	// Truncated to microseconds (the timestamptz resolution) so the stored
	// calculated_at is exactly the instant the score was computed with —
	// tests recompute the score from the stored rows and this timestamp.
	now := time.Now().Truncate(time.Microsecond)
	score := domain.CalculateBurgerScore(facts, now)
	if _, err := q.UpsertBurgerStats(ctx, sqlcgen.UpsertBurgerStatsParams{
		BurgerID:      burgerID,
		ReviewCount:   int64(len(facts)),
		AverageRating: domain.AverageRating(facts),
		WeightedScore: score.WeightedAverage,
		Confidence:    score.Confidence,
		CalculatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("recalculate burger stats: upsert: %w", err)
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
	if row.PhotoKey.Valid {
		key := row.PhotoKey.String
		review.PhotoKey = &key
	}
	return review
}

// toReviewDetail maps the joined review columns (shared by the list and
// detail queries) onto the domain payload; absent stats become zeros.
func toReviewDetail(
	id int64, rating int16, comment, photoKey pgtype.Text, createdAt pgtype.Timestamptz,
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
	if photoKey.Valid {
		key := photoKey.String
		detail.PhotoKey = &key
	}
	return detail
}
