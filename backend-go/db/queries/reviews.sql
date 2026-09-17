-- name: CreateReview :one
INSERT INTO reviews (rating, comment, user_id, burger_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetReview :one
SELECT * FROM reviews
WHERE id = $1;

-- name: ListReviews :many
SELECT * FROM reviews
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: UpdateReview :one
UPDATE reviews
SET rating = $2,
    comment = $3,
    discarded_at = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteReview :exec
DELETE FROM reviews
WHERE id = $1;
