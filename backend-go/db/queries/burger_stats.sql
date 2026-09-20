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

-- name: GetBurgerStats :one
SELECT * FROM burger_stats
WHERE burger_id = $1;

-- name: DeleteBurgerStats :exec
DELETE FROM burger_stats
WHERE burger_id = $1;
