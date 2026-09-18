-- name: UpsertBurgerStats :one
INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (burger_id) DO UPDATE
SET review_count = EXCLUDED.review_count,
    average_rating = EXCLUDED.average_rating,
    weighted_score = EXCLUDED.weighted_score,
    confidence = EXCLUDED.confidence,
    calculated_at = EXCLUDED.calculated_at
RETURNING *;

-- name: LockBurgerForStats :one
-- Serializes burger_stats recalculation per burger. Recalculation is
-- read-all-then-overwrite, so under READ COMMITTED two concurrent
-- transactions could each read a snapshot missing the other's uncommitted
-- review and the later upsert would overwrite the stats with a stale count
-- (lost update). FOR UPDATE on the burgers row makes the second
-- transaction block here until the first commits; its next statement then
-- sees the committed review.
SELECT id FROM burgers
WHERE id = $1
FOR UPDATE;

-- name: ListBurgerReviewFacts :many
-- The kept reviews feeding one burger's stats: excludes discarded reviews
-- AND reviews of discarded users (issue #15 R4/AC4 — deliberately stricter
-- than Rails' burger.reviews.kept, per the story decision). No active-shop
-- filter: stats aggregate all kept reviews, mirroring Rails.
SELECT r.rating, r.created_at, r.user_id
FROM reviews r
JOIN users u ON u.id = r.user_id
WHERE r.burger_id = $1
  AND r.discarded_at IS NULL
  AND u.discarded_at IS NULL
ORDER BY r.id;

-- name: ListReviewerRatings :many
-- Reviewer-trust history: each reviewer's kept ratings across ALL burgers
-- (mirrors Rails user.reviews.kept).
SELECT r.user_id, r.rating
FROM reviews r
WHERE r.user_id = ANY(sqlc.arg(user_ids)::bigint[])
  AND r.discarded_at IS NULL
ORDER BY r.id;

-- name: GetBurgerStats :one
SELECT * FROM burger_stats
WHERE burger_id = $1;

-- name: DeleteBurgerStats :exec
DELETE FROM burger_stats
WHERE burger_id = $1;
