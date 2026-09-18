-- name: CreateUser :one
INSERT INTO users (email, username, password_digest, admin)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1;

-- name: GetActiveUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND discarded_at IS NULL;

-- name: GetActiveUserByID :one
SELECT * FROM users
WHERE id = $1 AND discarded_at IS NULL;

-- name: ListActiveUsers :many
-- User index: every kept user, id ascending. No LIMIT/OFFSET — Rails
-- parity: the index returns all kept users unpaginated.
SELECT * FROM users
WHERE discarded_at IS NULL
ORDER BY id;

-- name: UpdateUserUsername :one
-- Column-scoped profile update: touches only username, and only while
-- the user is still kept, so a concurrent email/password change or soft
-- delete is never reverted from a stale snapshot.
UPDATE users
SET username = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: UpdateUserEmail :one
-- Column-scoped profile update: touches only email (see
-- UpdateUserUsername for the rationale).
UPDATE users
SET email = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: UpdateUserPasswordDigest :one
-- Column-scoped profile update: touches only password_digest (see
-- UpdateUserUsername for the rationale).
UPDATE users
SET password_digest = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: DiscardUser :one
-- Column-scoped soft delete: only stamps discarded_at, and only once —
-- an already-discarded user matches no row, surfacing as not found
-- (mirrors DiscardReview).
UPDATE users
SET discarded_at = now(),
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING id;
