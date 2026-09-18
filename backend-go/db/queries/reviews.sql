-- name: CreateReview :one
INSERT INTO reviews (rating, comment, user_id, burger_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPublicReviews :many
-- Global review feed: non-discarded reviews whose burger is served by at
-- least one active shop (status 1), with author, burger, and stats in one
-- query (no N+1). EXISTS instead of a plain JOIN on shops_burgers so a
-- burger linked to several active shops still yields exactly one row.
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN users u ON u.id = r.user_id
JOIN burgers b ON b.id = r.burger_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE r.discarded_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM shops_burgers sb
      JOIN shops s ON s.id = sb.shop_id
      WHERE sb.burger_id = r.burger_id AND s.status = 1
  )
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetReviewDetail :one
-- One non-discarded review with author, burger, and stats — serves both
-- the public detail endpoint and the load-for-authorization of edit and
-- delete (user_id carries the ownership check).
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN users u ON u.id = r.user_id
JOIN burgers b ON b.id = r.burger_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE r.id = $1 AND r.discarded_at IS NULL;

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

-- name: DiscardReview :execrows
-- Column-scoped soft delete: only stamps discarded_at, and only once —
-- an already-discarded review matches no row, surfacing as not found.
UPDATE reviews
SET discarded_at = now(),
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL;
