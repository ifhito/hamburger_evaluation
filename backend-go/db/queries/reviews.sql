-- name: CreateReview :one
INSERT INTO reviews (rating, comment, user_id, burger_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPublicReviews :many
-- Global review feed: non-discarded reviews of non-discarded users whose
-- burger is served by at least one active shop (status 1), with author,
-- burger, and stats in one query (no N+1). EXISTS instead of a plain JOIN
-- on shops_burgers so a burger linked to several active shops still
-- yields exactly one row. The u.discarded_at filter hides discarded
-- users' (still kept) reviews from the feed (S8).
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN users u ON u.id = r.user_id
JOIN burgers b ON b.id = r.burger_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE r.discarded_at IS NULL
  AND u.discarded_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM shops_burgers sb
      JOIN shops s ON s.id = sb.shop_id
      WHERE sb.burger_id = r.burger_id AND s.status = 1
  )
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetReviewDetail :one
-- One non-discarded review of a non-discarded user with author, burger,
-- and stats — serves both the public detail endpoint and the
-- load-for-authorization of edit and delete (user_id carries the
-- ownership check). A discarded author makes the review indistinguishable
-- from a missing one (S8).
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN users u ON u.id = r.user_id
JOIN burgers b ON b.id = r.burger_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE r.id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL;

-- name: ListUserKeptReviewBurgerIDs :many
-- The distinct burgers the user's kept reviews touch, for the S8
-- user-discard stats recalculation. The ascending burger_id ORDER BY is
-- load-bearing: recalculateBurgerStats locks each burger FOR UPDATE, and
-- all multi-burger callers must lock in ascending burger_id order so
-- overlapping burger sets cannot deadlock.
SELECT DISTINCT burger_id FROM reviews
WHERE user_id = $1 AND discarded_at IS NULL
ORDER BY burger_id;

-- name: UpdateReviewContent :one
-- Column-scoped edit: touches only rating and comment (never
-- discarded_at), and only while the review is still kept, so an edit can
-- neither resurrect nor race a concurrent soft delete.
UPDATE reviews
SET rating = $2,
    comment = $3,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: DiscardReview :one
-- Column-scoped soft delete: only stamps discarded_at, and only once —
-- an already-discarded review matches no row, surfacing as not found.
-- Returns burger_id so the caller can recalculate that burger's stats in
-- the same transaction.
UPDATE reviews
SET discarded_at = now(),
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING burger_id;
